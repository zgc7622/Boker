package bind

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	_ "reflect"
	"time"

	"github.com/Bokerchain/Boker/chain"
	"github.com/Bokerchain/Boker/chain/accounts/abi"
	"github.com/Bokerchain/Boker/chain/boker/protocol"
	"github.com/Bokerchain/Boker/chain/common"
	"github.com/Bokerchain/Boker/chain/core/types"
	"github.com/Bokerchain/Boker/chain/crypto"
	"github.com/Bokerchain/Boker/chain/eth"
	"github.com/Bokerchain/Boker/chain/log"
	"github.com/Bokerchain/Boker/chain/node"
)

// SignerFn is a signer function callback when a contract requires a method to
// sign the transaction before submission.
type SignerFn func(types.Signer, common.Address, *types.Transaction) (*types.Transaction, error)

// CallOpts is the collection of options to fine tune a contract call request.
type CallOpts struct {
	Pending bool            // Whether to operate on the pending state or the last known one
	From    common.Address  // Optional the sender address, otherwise the first account is used
	Context context.Context // Network context to support cancellation and timeouts (nil = no timeout)
}

//创建一个有效的以太坊交易
type TransactOpts struct {
	From     common.Address  // Ethereum account to send the transaction from
	Nonce    *big.Int        // Nonce to use for the transaction execution (nil = use pending state)
	Signer   SignerFn        // Method to use for signing the transaction (mandatory)
	Value    *big.Int        // Funds to transfer along along the transaction (nil = 0 = no funds)
	GasPrice *big.Int        // Gas price to use for the transaction execution (nil = gas price oracle)
	GasLimit *big.Int        // Gas limit to set for the transaction execution (nil = estimate + 10%)
	Context  context.Context // Network context to support cancellation and timeouts (nil = no timeout)
}

//BoundContract定义以太坊合约的基础包装器对象 它包含一组由方法使用的方法更高级别的合同绑定操作。
type BoundContract struct {
	address    common.Address     // Deployment address of the contract on the Ethereum blockchain
	abi        abi.ABI            // Reflect based ABI to access the correct Ethereum methods
	caller     ContractCaller     // Read interface to interact with the blockchain
	transactor ContractTransactor // Write interface to interact with the blockchain
}

var GethNode *node.Node

//NewBoundContract 创建一个通过其调用的低级合约接口并且交易可以通过。
func NewBoundContract(address common.Address,
	abi abi.ABI,
	caller ContractCaller,
	transactor ContractTransactor) *BoundContract {
	return &BoundContract{
		address:    address,
		abi:        abi,
		caller:     caller,
		transactor: transactor,
	}
}

func DeployContract(opts *TransactOpts, abi abi.ABI, bytecode []byte, backend ContractBackend, params ...interface{}) (common.Address, *types.Transaction, *BoundContract, error) {

	log.Info("****DeployContract****")

	//赋值
	c := NewBoundContract(common.Address{}, abi, backend, backend)

	input, err := c.abi.Pack("", params...)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	tx, err := c.transact(opts, nil, append(bytecode, input...), []byte(""), protocol.Binary)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	c.address = crypto.CreateAddress(opts.From, tx.Nonce())
	return c.address, tx, c, nil
}

//调用合约方法，并将params作为输入值和将输出设置为result
func (c *BoundContract) Call(opts *CallOpts, result interface{}, method string, params ...interface{}) error {

	//判断opts是否为空
	if opts == nil {
		opts = new(CallOpts)
	}

	//打包输入，调用并解压缩结果
	input, err := c.abi.Pack(method, params...)
	if err != nil {
		return err
	}

	var (
		msg    = ethereum.CallMsg{From: opts.From, To: &c.address, Data: input}
		ctx    = ensureContext(opts.Context)
		code   []byte
		output []byte
	)

	if opts.Pending {

		pb, ok := c.caller.(PendingContractCaller)
		if !ok {
			return ErrNoPendingState
		}

		output, err = pb.PendingCallContract(ctx, msg)
		if err == nil && len(output) == 0 {
			// Make sure we have a contract to operate on, and bail out otherwise.
			if code, err = pb.PendingCodeAt(ctx, c.address); err != nil {
				return err
			} else if len(code) == 0 {
				return ErrNoCode
			}
		}

	} else {

		output, err = c.caller.CallContract(ctx, msg, nil)
		if err == nil && len(output) == 0 {

			log.Info("Call", "outputlength", len(output))

			// Make sure we have a contract to operate on, and bail out otherwise.
			if code, err = c.caller.CodeAt(ctx, c.address, nil); err != nil {
				return err
			} else if len(code) == 0 {
				return ErrNoCode
			}
		}

	}
	if err != nil {
		return err
	}
	return c.abi.Unpack(result, method, output)
}

//得到当前分币帐号
func (c *BoundContract) getTokenNoder(opts *TransactOpts) (common.Address, error) {

	var ether *eth.Ethereum
	if err := GethNode.Service(&ether); err != nil {
		return common.Address{}, err
	}

	if ether.BlockChain().CurrentBlock() == nil {
		return common.Address{}, errors.New("failed to lookup token node")
	}

	firstTimer := ether.BlockChain().GetBlockByNumber(0).Time().Int64()
	return ether.BlockChain().CurrentBlock().DposCtx().GetCurrentTokenNoder(firstTimer)
}

//得到当前分币帐号
func (c *BoundContract) getNowTokenNoder(opts *TransactOpts, now int64) (common.Address, error) {

	var ether *eth.Ethereum
	if err := GethNode.Service(&ether); err != nil {
		return common.Address{}, err
	}

	if ether.BlockChain().CurrentBlock() == nil {
		return common.Address{}, errors.New("failed to lookup token node")
	}

	firstTimer := ether.BlockChain().GetBlockByNumber(0).Time().Int64()
	return ether.BlockChain().CurrentBlock().DposCtx().GetNowTokenNoder(firstTimer, now)
}

//得到当前的验证者帐号
func (c *BoundContract) getProducer(opts *TransactOpts) (common.Address, error) {

	var ether *eth.Ethereum
	if err := GethNode.Service(&ether); err != nil {
		return common.Address{}, err
	}

	if ether.BlockChain().CurrentBlock() == nil {
		return common.Address{}, errors.New("failed to lookup token node")
	}

	firstTimer := ether.BlockChain().GetBlockByNumber(0).Time().Int64()
	return ether.BlockChain().CurrentBlock().DposCtx().GetCurrentProducer(firstTimer)
}

//得到参数类型
func typeof(v interface{}) bool {

	switch v.(type) {
	case string:
		return true
	default:
		return false
	}
}

func (c *BoundContract) Transact(opts *TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {

	var now = time.Now().Unix()

	log.Info("Create Transact", "method", method)

	//打包合约参数
	input, err := c.abi.Pack(method, params...)
	if err != nil {
		return nil, err
	}

	//判断节点是否已经启动
	if GethNode != nil {

		var e *eth.Ethereum
		if err := GethNode.Service(&e); err != nil {
			return nil, err
		}

		//得到合约类型
		contractType, err := e.Boker().GetContract(c.address)
		if err != nil {
			return nil, err
		}

		//判断合约类型是否是基础合约
		extra := []byte("")
		log.Info("Create Transact", "contractType", contractType)
		if contractType == protocol.PersonalContract {

			//用户触发的基础合约（用户触发，但是不收取Gas费用）
			if method == protocol.RegisterCandidateMethod {
				return c.transact(opts, &c.address, input, extra, protocol.RegisterCandidate)
			} else if method == protocol.VoteCandidateMethod {
				return c.transact(opts, &c.address, input, extra, protocol.VoteUser)
			} else if method == protocol.CancelVoteMethod {
				return c.transact(opts, &c.address, input, extra, protocol.VoteCancel)
			} else if method == protocol.FireEventMethod {
				return c.transact(opts, &c.address, input, extra, protocol.UserEvent)
			}
			return nil, errors.New("unknown personal contract method name")

		} else if contractType == protocol.SystemContract {

			//由基础链触发的基础合约，不收取Gas费用
			if method == protocol.AssignTokenMethod {

				//得到当前的分币节点
				tokennoder, err := c.getNowTokenNoder(opts, now)
				if err != nil {
					return nil, errors.New("get assign token error")
				}
				if tokennoder != opts.From {
					return nil, errors.New("current assign token not is from account")
				}

				return c.assginTransact(opts, &c.address, input, extra, protocol.AssignToken, now)

				//return c.transact(opts, &c.address, input, extra, protocol.AssignToken)

			} else if method == protocol.RotateVoteMethod {

				//得到当前的分币节点
				tokennoder, err := c.getTokenNoder(opts)
				if err != nil {
					return nil, errors.New("get rotate vote error")
				}
				if tokennoder != opts.From {
					return nil, errors.New("current rotate vote not is from account")
				}
				return c.transact(opts, &c.address, input, extra, protocol.VoteEpoch)
			}
			return nil, errors.New("unknown system contract method name")
		}
	}

	//测试使用
	if method == protocol.FireEventMethod {

		log.Info("(c *BoundContract) Transact", "FireEventMethod", protocol.FireEventMethod)
		extra := []byte("")
		return c.transact(opts, &c.address, input, extra, protocol.UserEvent)
	}

	return c.transact(opts, &c.address, input, []byte(""), protocol.Binary)
}

func (c *BoundContract) TryTransact(opts *TransactOpts, now int64, method string, params ...interface{}) (*types.Transaction, error) {

	log.Info("(c *BoundContract) TryTransact", "now", now, "method", method)

	input, err := c.abi.Pack(method, params...)
	if err != nil {
		return nil, err
	}

	if GethNode != nil {

		var e *eth.Ethereum
		if err := GethNode.Service(&e); err != nil {
			return nil, err
		}

		if method == protocol.AssignTokenMethod {

			tokennoder, err := c.getNowTokenNoder(opts, now)
			if err != nil {
				return nil, errors.New("get assign token error")
			}
			if tokennoder != opts.From {
				return nil, errors.New("current assign token not is from account")
			}
			return c.assginTransact(opts, &c.address, input, []byte(""), protocol.AssignToken, now)

		} else if method == protocol.RotateVoteMethod {

			tokennoder, err := c.getTokenNoder(opts)
			if err != nil {
				return nil, errors.New("get rotate vote error")
			}
			if tokennoder != opts.From {
				return nil, errors.New("current rotate vote not is from account")
			}
			return c.transact(opts, &c.address, input, []byte(""), protocol.VoteEpoch)
		} else {
			return nil, errors.New("unknown system contract method name")
		}
	}
	return nil, errors.New("node not run")
}

func (c *BoundContract) Transfer(opts *TransactOpts) (*types.Transaction, error) {

	log.Info("(c *BoundContract) Transfer")

	var e *eth.Ethereum
	if err := GethNode.Service(&e); err != nil {
		return nil, err
	}

	txType, err := e.Boker().GetContract(c.address)
	if err != nil {
		return nil, err
	}

	return c.transact(opts, &c.address, nil, []byte(""), protocol.TxType(txType))
}

func (c *BoundContract) baseTransact(opts *TransactOpts, contract *common.Address, payload []byte, extra []byte, transactTypes protocol.TxType) (*types.Transaction, error) {

	//判断Value值是否为空
	var err error
	value := opts.Value
	if value == nil {
		value = new(big.Int)
	}

	//判断Nonce值是否为空
	var nonce uint64
	if opts.Nonce == nil {
		nonce, err = c.transactor.PendingNonceAt(ensureContext(opts.Context), opts.From)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve account nonce: %v", err)
		}
		log.Info("(c *BoundContract) baseTransact1", "nonce", nonce)
	} else {
		nonce = opts.Nonce.Uint64()
		log.Info("(c *BoundContract) baseTransact2", "nonce", nonce)
	}
	log.Info("(c *BoundContract) baseTransact", "nonce", nonce)

	/*这里不对Gas和GasLimit进行设置，因为在函数NewBaseTransaction里面已经针对基础业务进行了设置*/
	var rawTx *types.Transaction
	if contract == nil {
		return nil, errors.New("not found base contract address")
	} else {
		rawTx = types.NewBaseTransaction(transactTypes, nonce, c.address, value, payload)
	}

	//判断交易是否有签名者
	if opts.Signer == nil {
		return nil, errors.New("no signer to authorize the transaction with")
	}

	//进行签名
	signedTx, err := opts.Signer(types.HomesteadSigner{}, opts.From, rawTx)
	if err != nil {
		return nil, err
	}

	//将交易注入pending池中
	if err := c.transactor.SendTransaction(ensureContext(opts.Context), signedTx); err != nil {
		return nil, err
	}
	return signedTx, nil
}

func (c *BoundContract) assginTransact(opts *TransactOpts, contract *common.Address, payload []byte, extra []byte, transactTypes protocol.TxType, now int64) (*types.Transaction, error) {

	var err error
	value := opts.Value
	if value == nil {
		value = new(big.Int)
	}

	var nonce uint64
	if opts.Nonce == nil {
		nonce, err = c.transactor.PendingNonceAt(ensureContext(opts.Context), opts.From)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve account nonce: %v", err)
		}
		log.Info("(c *BoundContract) baseTransact1", "nonce", nonce)
	} else {
		nonce = opts.Nonce.Uint64()
		log.Info("(c *BoundContract) baseTransact2", "nonce", nonce)
	}
	log.Info("(c *BoundContract) baseTransact", "nonce", nonce)

	var rawTx *types.Transaction
	if contract == nil {
		return nil, errors.New("not found base contract address")
	} else {
		rawTx = types.NewAssginTransaction(transactTypes, nonce, c.address, value, payload, now)
	}

	if opts.Signer == nil {
		return nil, errors.New("no signer to authorize the transaction with")
	}

	signedTx, err := opts.Signer(types.HomesteadSigner{}, opts.From, rawTx)
	if err != nil {
		return nil, err
	}

	if err := c.transactor.SendTransaction(ensureContext(opts.Context), signedTx); err != nil {
		return nil, err
	}
	return signedTx, nil
}

func (c *BoundContract) normalTransact(opts *TransactOpts, contract *common.Address, payload []byte, extra []byte, transactTypes protocol.TxType) (*types.Transaction, error) {

	log.Info("(c *BoundContract) normalTransact", "from", opts.From)

	//判断Value值是否为空
	var err error
	value := opts.Value
	if value == nil {
		value = new(big.Int)
	}

	//判断Nonce值是否为空
	var nonce uint64
	if opts.Nonce == nil {

		//如果Nonce值为空，则初始化一个nonce值来进行初始化
		nonce, err = c.transactor.PendingNonceAt(ensureContext(opts.Context), opts.From)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve account nonce: %v", err)
		}
	} else {
		nonce = opts.Nonce.Uint64()
	}
	log.Info("(c *BoundContract) normalTransact", "from", opts.From, "nonce", nonce)

	//如果GasPrice为空，则设置一个建议的GasPrice
	gasPrice := opts.GasPrice
	if gasPrice == nil {
		gasPrice, err = c.transactor.SuggestGasPrice(ensureContext(opts.Context)) //得到一个建议的GasPrice
		if err != nil {
			return nil, fmt.Errorf("failed to suggest gas price: %v", err)
		}
	}

	//如果GasLimit为空，则设置一个GasLimit
	gasLimit := opts.GasLimit
	if gasLimit == nil {

		//如果合约存在，则根据合约内容评估一个GasLimit
		if contract != nil {

			if code, err := c.transactor.PendingCodeAt(ensureContext(opts.Context), c.address); err != nil {
				return nil, err
			} else if len(code) == 0 {
				return nil, ErrNoCode
			}
		}

		//估算所需要的Gas
		msg := ethereum.CallMsg{From: opts.From, To: contract, Value: value, Data: payload, Extra: extra}
		gasLimit, err = c.transactor.EstimateGas(ensureContext(opts.Context), msg)
		if err != nil {
			return nil, fmt.Errorf("failed to estimate gas needed: %v", err) //估算所需gas失败
		}
	}

	//创建合约交易或者直接产生一个交易
	var rawTx *types.Transaction
	if contract == nil {
		//如果合约尚未创建，则创建合约
		rawTx = types.NewContractCreation(nonce, value, gasLimit, gasPrice, payload)
	} else {
		//合约已经创建，则创建一个交易
		rawTx = types.NewTransaction(transactTypes, nonce, c.address, value, gasLimit, gasPrice, payload)
	}

	//判断交易是否有签名者
	if opts.Signer == nil {
		return nil, errors.New("no signer to authorize the transaction with")
	}

	//进行签名
	signedTx, err := opts.Signer(types.HomesteadSigner{}, opts.From, rawTx)
	if err != nil {
		return nil, err
	}

	//将交易注入pending池中
	//log.Info("****c.transactor.SendTransaction****")
	if err := c.transactor.SendTransaction(ensureContext(opts.Context), signedTx); err != nil {
		return nil, err
	}
	return signedTx, nil
}

func (c *BoundContract) transact(opts *TransactOpts, contract *common.Address, payload []byte, extra []byte, transactTypes protocol.TxType) (*types.Transaction, error) {

	/*根据不同类型计算使用的Gas信息*/
	if transactTypes == protocol.Binary {

		//普通交易
		return c.normalTransact(opts, contract, payload, extra, transactTypes)
	} else if (transactTypes >= protocol.SetValidator) && (transactTypes <= protocol.AssignToken) {

		//基础合约交易
		return c.baseTransact(opts, contract, payload, extra, transactTypes)
	} else {

		//未知的类型
		log.Error("transact", "transactTypes", transactTypes)
		return nil, errors.New("unknown transaction type")
	}
}

//
func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.TODO()
	}
	return ctx
}
