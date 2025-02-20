package params

import "math/big"

const (
	MaximumExtraDataSize    uint64 = 32     // Maximum size extra data may be after Genesis.
	ExpByteGas              uint64 = 10     // Times ceil(log256(exponent)) for the EXP instruction.
	SloadGas                uint64 = 50     // Multiplied by the number of 32-byte words that are copied (round up) for any *COPY operation and added.
	CallValueTransferGas    uint64 = 9000   // Paid for CALL when the value transfer is non-zero.
	CallNewAccountGas       uint64 = 25000  // Paid for CALL when the destination address didn't exist prior.
	TxGas                   uint64 = 21000  // Per transaction not creating a contract. NOTE: Not payable on data of calls between transactions.
	TxGasContractCreation   uint64 = 53000  // Per transaction that creates a contract. NOTE: Not payable on data of calls between transactions.
	TxDataZeroGas           uint64 = 4      // Per byte of data attached to a transaction that equals zero. NOTE: Not payable on data of calls between transactions.
	QuadCoeffDiv            uint64 = 512    // Divisor for the quadratic particle of the memory cost equation.
	SstoreSetGas            uint64 = 20000  // Once per SLOAD operation.
	LogDataGas              uint64 = 8      // Per byte in a LOG* operation's data.
	CallStipend             uint64 = 2300   // Free gas given at beginning of call.
	Sha3Gas                 uint64 = 30     // Once per SHA3 operation.
	Sha3WordGas             uint64 = 6      // Once per word of the SHA3 operation's data.
	SstoreResetGas          uint64 = 5000   // Once per SSTORE operation if the zeroness changes from zero.
	SstoreClearGas          uint64 = 5000   // Once per SSTORE operation if the zeroness doesn't change.
	SstoreRefundGas         uint64 = 15000  // Once per SSTORE operation if the zeroness changes to zero.
	JumpdestGas             uint64 = 1      // Refunded gas, once per SSTORE operation if the zeroness changes to zero.
	EpochDuration           uint64 = 30000  // Duration between proof-of-work epochs.
	CallGas                 uint64 = 40     // Once per CALL operation & message call transaction.
	CreateDataGas           uint64 = 200    //
	CreateBaseContractGas   uint64 = 0      //
	CallCreateDepth         uint64 = 1024   // Maximum depth of call/create stack.
	ExpGas                  uint64 = 10     // Once per EXP instruction
	LogGas                  uint64 = 375    // Per LOG* operation.
	CopyGas                 uint64 = 3      //
	StackLimit              uint64 = 1024   // Maximum size of VM stack allowed.
	TierStepGas             uint64 = 0      // Once per operation, for a selection of them.
	LogTopicGas             uint64 = 375    // Multiplied by the * of the LOG*, per LOG transaction. e.g. LOG0 incurs 0 * c_txLogTopicGas, LOG4 incurs 4 * c_txLogTopicGas.
	CreateGas               uint64 = 32000  // Once per CREATE operation & contract-creation transaction.
	SuicideRefundGas        uint64 = 24000  // Refunded following a suicide operation.
	MemoryGas               uint64 = 3      // Times the address of the (highest referenced byte in memory + 1). NOTE: referencing happens on read, write and in instructions such as RETURN and CALL.
	TxDataNonZeroGas        uint64 = 68     // Per byte of data attached to a transaction that is not equal to zero. NOTE: Not payable on data of calls between transactions.
	MaxCodeSize                    = 24576  // 允许合约的最大字节码(24K)
	EcrecoverGas            uint64 = 3000   // Elliptic curve sender recovery gas price
	Sha256BaseGas           uint64 = 60     // Base price for a SHA256 operation
	Sha256PerWordGas        uint64 = 12     // Per-word price for a SHA256 operation
	Ripemd160BaseGas        uint64 = 600    // Base price for a RIPEMD160 operation
	Ripemd160PerWordGas     uint64 = 120    // Per-word price for a RIPEMD160 operation
	IdentityBaseGas         uint64 = 15     // Base price for a data copy operation
	IdentityPerWordGas      uint64 = 3      // Per-work price for a data copy operation
	ModExpQuadCoeffDiv      uint64 = 20     // Divisor for the quadratic particle of the big int modular exponentiation
	Bn256AddGas             uint64 = 500    // Gas needed for an elliptic curve addition
	Bn256ScalarMulGas       uint64 = 40000  // Gas needed for an elliptic curve scalar multiplication
	Bn256PairingBaseGas     uint64 = 100000 // Base price for an elliptic curve pairing check
	Bn256PairingPerPointGas uint64 = 80000  // Per-point price for an elliptic curve pairing check
)

var (
	GasLimitSsthresh       = big.NewFloat(1e+18)                //最大的Gas极限1Bobby
	GasLimitBoundDivisor   = big.NewInt(1024)                   //Gas限制的约束除数，用于更新计算
	MinGasLimit            = big.NewInt(5000)                   //区块最小的Gas限制
	MinGasLimitFloat       = new(big.Float).SetInt(MinGasLimit) //区块的最小Gas
	GenesisGasLimit        = big.NewInt(4712388)                //Genesis块的Gas限制.
	TargetGasLimit         = new(big.Int).Set(GenesisGasLimit)  // The artificial target
	DifficultyBoundDivisor = big.NewInt(2048)                   // The bound divisor of the difficulty, used in the update calculations.
	GenesisDifficulty      = big.NewInt(131072)                 // Difficulty of the Genesis block.
	MinimumDifficulty      = big.NewInt(131072)                 // The minimum that the difficulty may ever be.
	DurationLimit          = big.NewInt(13)                     // The decision boundary on the blocktime duration used to determine whether difficulty should go up or not.
	BlockSize              = big.NewInt(10 * 1024 * 1024)       //播客链新增区块大小限制 10MB
)
