package vm

import (
	"fmt"
	"sync/atomic"

	"github.com/Bokerchain/Boker/chain/common"
	"github.com/Bokerchain/Boker/chain/common/math"
	"github.com/Bokerchain/Boker/chain/crypto"
	_ "github.com/Bokerchain/Boker/chain/log"
	"github.com/Bokerchain/Boker/chain/params"
)

// Config are the configuration options for the Interpreter
type Config struct {
	Debug     bool //启用调试调试解释器选项
	EnableJit bool //启用Jit虚拟机
	ForceJit  bool //强制JIT虚拟机
	// Tracer is the op code logger
	Tracer Tracer
	// NoRecursion disabled Interpreter call, callcode,
	// delegate call and create.
	NoRecursion bool
	// Disable gas metering
	DisableGasMetering bool
	// Enable recording of SHA3/keccak preimages
	EnablePreimageRecording bool
	// JumpTable contains the EVM instruction table. This
	// may be left uninitialised and will be set to the default
	// table.
	JumpTable [256]operation
}

// Interpreter is used to run Ethereum based contracts and will utilise the
// passed evmironment to query external sources for state information.
// The Interpreter will run the byte code VM or JIT VM based on the passed
// configuration.
type Interpreter struct {
	evm        *EVM
	cfg        Config
	gasTable   params.GasTable
	intPool    *intPool
	readOnly   bool   // Whether to throw on stateful modifications
	returnData []byte // Last CALL's return data for subsequent reuse
}

// NewInterpreter returns a new instance of the Interpreter.
func NewInterpreter(evm *EVM, cfg Config) *Interpreter {
	// We use the STOP instruction whether to see
	// the jump table was initialised. If it was not
	// we'll set the default jump table.
	/*if !cfg.JumpTable[STOP].valid {
		switch {
		case evm.ChainConfig().IsByzantium(evm.BlockNumber):
			cfg.JumpTable = byzantiumInstructionSet
		case evm.ChainConfig().IsHomestead(evm.BlockNumber):
			cfg.JumpTable = homesteadInstructionSet
		default:
			cfg.JumpTable = frontierInstructionSet
		}
	}*/
	if !cfg.JumpTable[STOP].valid {
		cfg.JumpTable = homesteadInstructionSet
	}

	return &Interpreter{
		evm:      evm,
		cfg:      cfg,
		gasTable: evm.ChainConfig().GasTable(evm.BlockNumber),
		intPool:  newIntPool(),
	}
}

func (in *Interpreter) enforceRestrictions(op OpCode, operation operation, stack *Stack) error {
	if in.evm.chainRules.IsByzantium {
		if in.readOnly {
			// If the interpreter is operating in readonly mode, make sure no
			// state-modifying operation is performed. The 3rd stack item
			// for a call operation is the value. Transferring value from one
			// account to the others means the state is modified and should also
			// return with an error.
			if operation.writes || (op == CALL && stack.Back(2).BitLen() > 0) {
				return errWriteProtection
			}
		}
	}
	return nil
}

//执行智能合约
func (in *Interpreter) Run(snapshot int, contract *Contract, input []byte) (ret []byte, err error) {

	//增加调用深度，限制为1024
	in.evm.depth++
	defer func() { in.evm.depth-- }()

	// Reset the previous call's return data. It's unimportant to preserve the old buffer
	// as every returning call will return new data anyway.
	in.returnData = nil

	//判断合约是否存在代码
	if len(contract.Code) == 0 {
		return nil, nil
	}

	codehash := contract.CodeHash // codehash is used when doing jump dest caching
	if codehash == (common.Hash{}) {
		codehash = crypto.Keccak256Hash(contract.Code)
	}

	var (
		op    OpCode        // current opcode
		mem   = NewMemory() // bound memory
		stack = newstack()  // local stack
		// For optimisation reason we're using uint64 as the program counter.
		// It's theoretically possible to go above 2^64. The YP defines the PC
		// to be uint256. Practically much less so feasible.
		pc   = uint64(0) // program counter
		cost uint64
		// copies used by tracer
		stackCopy = newstack() // stackCopy needed for Tracer since stack is mutated by 63/64 gas rule
		pcCopy    uint64       // needed for the deferred Tracer
		gasCopy   uint64       // for Tracer to log gas remaining before execution
		logged    bool         // deferred Tracer should ignore already logged steps
	)
	contract.Input = input

	defer func() {
		if err != nil && !logged && in.cfg.Debug {
			in.cfg.Tracer.CaptureState(in.evm, pcCopy, op, gasCopy, cost, mem, stackCopy, contract, in.evm.depth, err)
		}
	}()

	// The Interpreter main run loop (contextual). This loop runs until either an
	// explicit STOP, RETURN or SELFDESTRUCT is executed, an error occurred during
	// the execution of one of the operations or until the done flag is set by the
	// parent context.

	//codeTest := contract.Code
	//log.Info("Run ", "input", input, "codeTest", codeTest)

	//监听about信号
	for atomic.LoadInt32(&in.evm.abort) == 0 {

		//opByte := byte(op)
		//log.Info("Run LoadInt32", "op", op, "pc", pc, "opByte", opByte)

		//pc是程序计数器，控制当前执行到的code位置, 正常情况下每次都会定位在操作码上
		op = contract.GetOp(pc)

		if in.cfg.Debug {
			logged = false
			pcCopy = pc
			gasCopy = contract.Gas
			stackCopy = newstack()
			for _, val := range stack.data {
				stackCopy.push(val)
			}
		}

		// Get the operation from the jump table matching the opcode and validate the
		// stack and make sure there enough stack items available to perform the operation
		operation := in.cfg.JumpTable[op]
		if !operation.valid {

			return nil, fmt.Errorf("invalid opcode 0x%x", int(op))
		}
		if err := operation.validateStack(stack); err != nil {

			return nil, err
		}
		// If the operation is valid, enforce and write restrictions
		if err := in.enforceRestrictions(op, operation, stack); err != nil {

			return nil, err
		}

		var memorySize uint64
		// calculate the new memory size and expand the memory to fit
		// the operation
		if operation.memorySize != nil {
			memSize, overflow := bigUint64(operation.memorySize(stack))
			if overflow {

				return nil, errGasUintOverflow
			}
			// memory is expanded in words of 32 bytes. Gas
			// is also calculated in words.
			if memorySize, overflow = math.SafeMul(toWordSize(memSize), 32); overflow {

				//log.Info("Run SafeMul", "err", errGasUintOverflow, "memorySize", memorySize)
				return nil, errGasUintOverflow
			}
		}

		if !in.cfg.DisableGasMetering {
			// consume the gas and return an error if not enough gas is available.
			// cost is explicitly set so that the capture state defer method cas get the proper cost
			cost, err = operation.gasCost(in.gasTable, in.evm, contract, stack, mem, memorySize)
			if err != nil || !contract.UseGas(cost) {

				//log.Info("Run gasCost", "err", ErrOutOfGas, "cost", cost)
				return nil, ErrOutOfGas
			}
		}
		if memorySize > 0 {
			mem.Resize(memorySize)
		}

		if in.cfg.Debug {
			in.cfg.Tracer.CaptureState(in.evm, pc, op, gasCopy, cost, mem, stackCopy, contract, in.evm.depth, err)
			logged = true
		}

		// execute the operation
		res, err := operation.execute(&pc, in.evm, contract, mem, stack)
		// verifyPool is a build flag. Pool verification makes sure the integrity
		// of the integer pool by comparing values to a default value.
		if verifyPool {
			verifyIntegerPool(in.intPool)
		}
		// if the operation clears the return data (e.g. it has returning data)
		// set the last return to the result of the operation.
		if operation.returns {
			in.returnData = res
		}

		switch {
		case err != nil:

			return nil, err
		case operation.reverts:

			//log.Info("Run reverts", "op", op, "pc", pc)
			return res, errExecutionReverted
		case operation.halts:

			//log.Info("Run halts", "op", op, "pc", pc)
			return res, nil
		case !operation.jumps:

			pc++
		}
	}

	return nil, nil
}
