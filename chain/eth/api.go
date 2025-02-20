package eth

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/Bokerchain/Boker/chain/common"
	"github.com/Bokerchain/Boker/chain/common/hexutil"
	"github.com/Bokerchain/Boker/chain/core"
	"github.com/Bokerchain/Boker/chain/core/state"
	"github.com/Bokerchain/Boker/chain/core/types"
	"github.com/Bokerchain/Boker/chain/core/vm"
	"github.com/Bokerchain/Boker/chain/internal/ethapi"
	"github.com/Bokerchain/Boker/chain/log"
	"github.com/Bokerchain/Boker/chain/params"
	"github.com/Bokerchain/Boker/chain/rlp"
	"github.com/Bokerchain/Boker/chain/rpc"
	"github.com/Bokerchain/Boker/chain/trie"
)

const defaultTraceTimeout = 5 * time.Second

var (
	ErrBlockChain   = errors.New("bokerchain error")    //区块错误
	ErrCurrentBlock = errors.New("current block error") //当前区块错误
	ErrDpos         = errors.New("current Dpos error")  //当前Dpos错误
)

//提供了访问以太网完全节点相关的API信息
type PublicEthereumAPI struct {
	e *Ethereum
}

//创建一个新的完整节点以太坊协议API
func NewPublicEthereumAPI(e *Ethereum) *PublicEthereumAPI {
	return &PublicEthereumAPI{e}
}

//得到当前验证者
func (api *PublicEthereumAPI) Validator() (common.Address, error) {

	if api.e.BlockChain() == nil {
		return common.Address{}, ErrBlockChain
	}
	if api.e.BlockChain().CurrentBlock() == nil {
		return common.Address{}, ErrCurrentBlock
	}
	if api.e.BlockChain().CurrentBlock().DposCtx() == nil {
		return common.Address{}, ErrDpos
	}

	firstTimer := api.e.BlockChain().GetBlockByNumber(0).Time().Int64()
	return api.e.BlockChain().CurrentBlock().DposCtx().GetCurrentProducer(firstTimer)
}

//采矿奖励将被发送到的地址（即挖矿者账号）
func (api *PublicEthereumAPI) Coinbase() (common.Address, error) {
	return api.e.Coinbase()
}

// Hashrate returns the POW hashrate
func (api *PublicEthereumAPI) Hashrate() hexutil.Uint64 {
	return hexutil.Uint64(api.e.Miner().HashRate())
}

//提供用来控制矿工的API，它仅提供对数据进行操作的方法，这些方法在可公开访问且不会带来安全风险。
type PublicMinerAPI struct {
	e *Ethereum
}

func NewPublicMinerAPI(e *Ethereum) *PublicMinerAPI {
	return &PublicMinerAPI{e}
}

//判断此节点是否当前正在挖矿
func (api *PublicMinerAPI) Mining() bool {
	return api.e.IsMining()
}

//提供用来控制矿工的私有RPC方法，由于这些方法可能被外部用户所滥用，所以必须被认为是不安全的，供不信任的用户使用。
type PrivateMinerAPI struct {
	e *Ethereum
}

func NewPrivateMinerAPI(e *Ethereum) *PrivateMinerAPI {
	return &PrivateMinerAPI{e: e}
}

//使用给定数量的线程启动矿工。 如果线程数为nil，则已启动的worker等于可用的逻辑CPU数,如果挖掘已在运行，则此方法会调整数量允许使用的线程
//使用指令启动挖矿
func (api *PrivateMinerAPI) Start(threads *int) error {
	// Set the number of threads if the seal engine supports it
	if threads == nil {
		threads = new(int)
	} else if *threads == 0 {
		*threads = -1 // Disable the miner from within
	}
	type threaded interface {
		SetThreads(threads int)
	}
	if th, ok := api.e.engine.(threaded); ok {
		log.Info("Updated mining threads", "threads", *threads)
		th.SetThreads(*threads)
	}
	// Start the miner and return
	if !api.e.IsMining() {
		// Propagate the initial price point to the transaction pool
		api.e.lock.RLock()
		price := api.e.gasPrice
		api.e.lock.RUnlock()

		api.e.txPool.SetGasPrice(price)
		return api.e.StartMining(true)
	}
	return nil
}

//停止矿工挖矿
func (api *PrivateMinerAPI) Stop() bool {
	type threaded interface {
		SetThreads(threads int)
	}
	if th, ok := api.e.engine.(threaded); ok {
		th.SetThreads(-1)
	}
	api.e.StopMining()
	return true
}

//设置此矿工挖掘块时包含的额外数据字符串
func (api *PrivateMinerAPI) SetExtra(extra string) (bool, error) {
	if err := api.e.Miner().SetExtra([]byte(extra)); err != nil {
		return false, err
	}
	return true, nil
}

//设置矿工的最低可接受Gas价格
func (api *PrivateMinerAPI) SetGasPrice(gasPrice hexutil.Big) bool {
	api.e.lock.Lock()
	api.e.gasPrice = (*big.Int)(&gasPrice)
	api.e.lock.Unlock()

	api.e.txPool.SetGasPrice((*big.Int)(&gasPrice))
	return true
}

//设置挖矿矿工账号
func (api *PrivateMinerAPI) SetCoinbase(coinbase common.Address) bool {

	api.e.SetCoinbase(coinbase)
	return true
}

//设置当前账号为本地节点的出块账号
func (api *PrivateMinerAPI) SetLocalValidator() bool {

	coinbase, err := api.e.Coinbase()
	if err != nil {
		return false
	}

	//log.Info("SetLocalValidator", "coinbase", coinbase.String())
	api.e.SetLocalValidator(coinbase)
	return true
}

// GetHashrate returns the current hashrate of the miner.
func (api *PrivateMinerAPI) GetHashrate() uint64 {
	return uint64(api.e.miner.HashRate())
}

//以太坊全节点相关API的集合，通过私有管理端点公开。
type PrivateAdminAPI struct {
	eth *Ethereum
}

func NewPrivateAdminAPI(eth *Ethereum) *PrivateAdminAPI {
	return &PrivateAdminAPI{eth: eth}
}

//将当前区块链导出到本地文件中
func (api *PrivateAdminAPI) ExportChain(file string) (bool, error) {
	// Make sure we can create the file to export into
	out, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return false, err
	}
	defer out.Close()

	var writer io.Writer = out
	if strings.HasSuffix(file, ".gz") {
		writer = gzip.NewWriter(writer)
		defer writer.(*gzip.Writer).Close()
	}

	// Export the blockchain
	if err := api.eth.BlockChain().Export(writer); err != nil {
		return false, err
	}
	return true, nil
}

func hasAllBlocks(chain *core.BlockChain, bs []*types.Block) bool {
	for _, b := range bs {
		if !chain.HasBlock(b.Hash(), b.NumberU64()) {
			return false
		}
	}

	return true
}

//从本地文件导入区块链
func (api *PrivateAdminAPI) ImportChain(file string) (bool, error) {
	// Make sure the can access the file to import
	in, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer in.Close()

	var reader io.Reader = in
	if strings.HasSuffix(file, ".gz") {
		if reader, err = gzip.NewReader(reader); err != nil {
			return false, err
		}
	}

	// Run actual the import in pre-configured batches
	stream := rlp.NewStream(reader, 0)

	blocks, index := make([]*types.Block, 0, 2500), 0
	for batch := 0; ; batch++ {
		// Load a batch of blocks from the input file
		for len(blocks) < cap(blocks) {
			block := new(types.Block)
			if err := stream.Decode(block); err == io.EOF {
				break
			} else if err != nil {
				return false, fmt.Errorf("block %d: failed to parse: %v", index, err)
			}
			blocks = append(blocks, block)
			index++
		}
		if len(blocks) == 0 {
			break
		}

		if hasAllBlocks(api.eth.BlockChain(), blocks) {
			blocks = blocks[:0]
			continue
		}
		// Import the batch and reset the buffer
		if _, err := api.eth.BlockChain().InsertChain(blocks); err != nil {
			return false, fmt.Errorf("batch %d: failed to insert: %v", batch, err)
		}
		blocks = blocks[:0]
	}
	return true, nil
}

//公开的以太坊全节点API，通过公共调试端点
type PublicDebugAPI struct {
	eth *Ethereum
}

func NewPublicDebugAPI(eth *Ethereum) *PublicDebugAPI {
	return &PublicDebugAPI{eth: eth}
}

// DumpBlock retrieves the entire state of the database at a given block.
func (api *PublicDebugAPI) DumpBlock(blockNr rpc.BlockNumber) (state.Dump, error) {
	if blockNr == rpc.PendingBlockNumber {
		// If we're dumping the pending state, we need to request
		// both the pending block as well as the pending state from
		// the miner and operate on those
		_, stateDb := api.eth.miner.Pending()
		return stateDb.RawDump(), nil
	}
	var block *types.Block
	if blockNr == rpc.LatestBlockNumber {
		block = api.eth.blockchain.CurrentBlock()
	} else {
		block = api.eth.blockchain.GetBlockByNumber(uint64(blockNr))
	}
	if block == nil {
		return state.Dump{}, fmt.Errorf("block #%d not found", blockNr)
	}
	stateDb, err := api.eth.BlockChain().StateAt(block.Root())
	if err != nil {
		return state.Dump{}, err
	}
	return stateDb.RawDump(), nil
}

//公开的以太坊全节点API，私有调试端点。
type PrivateDebugAPI struct {
	config *params.ChainConfig
	eth    *Ethereum
}

func NewPrivateDebugAPI(config *params.ChainConfig, eth *Ethereum) *PrivateDebugAPI {
	return &PrivateDebugAPI{config: config, eth: eth}
}

// BlockTraceResult is the returned value when replaying a block to check for
// consensus results and full VM trace logs for all included transactions.
type BlockTraceResult struct {
	Validated  bool                  `json:"validated"`
	StructLogs []ethapi.StructLogRes `json:"structLogs"`
	Error      string                `json:"error"`
}

// TraceArgs holds extra parameters to trace functions
type TraceArgs struct {
	*vm.LogConfig
	Tracer  *string
	Timeout *string
}

// TraceBlock processes the given block'api RLP but does not import the block in to
// the chain.
func (api *PrivateDebugAPI) TraceBlock(blockRlp []byte, config *vm.LogConfig) BlockTraceResult {
	var block types.Block
	err := rlp.Decode(bytes.NewReader(blockRlp), &block)
	if err != nil {
		return BlockTraceResult{Error: fmt.Sprintf("could not decode block: %v", err)}
	}

	validated, logs, err := api.traceBlock(&block, config)
	return BlockTraceResult{
		Validated:  validated,
		StructLogs: ethapi.FormatLogs(logs),
		Error:      formatError(err),
	}
}

// TraceBlockFromFile loads the block'api RLP from the given file name and attempts to
// process it but does not import the block in to the chain.
func (api *PrivateDebugAPI) TraceBlockFromFile(file string, config *vm.LogConfig) BlockTraceResult {
	blockRlp, err := ioutil.ReadFile(file)
	if err != nil {
		return BlockTraceResult{Error: fmt.Sprintf("could not read file: %v", err)}
	}
	return api.TraceBlock(blockRlp, config)
}

// TraceBlockByNumber processes the block by canonical block number.
func (api *PrivateDebugAPI) TraceBlockByNumber(blockNr rpc.BlockNumber, config *vm.LogConfig) BlockTraceResult {
	// Fetch the block that we aim to reprocess
	var block *types.Block
	switch blockNr {
	case rpc.PendingBlockNumber:
		// Pending block is only known by the miner
		block = api.eth.miner.PendingBlock()
	case rpc.LatestBlockNumber:
		block = api.eth.blockchain.CurrentBlock()
	default:
		block = api.eth.blockchain.GetBlockByNumber(uint64(blockNr))
	}

	if block == nil {
		return BlockTraceResult{Error: fmt.Sprintf("block #%d not found", blockNr)}
	}

	validated, logs, err := api.traceBlock(block, config)
	return BlockTraceResult{
		Validated:  validated,
		StructLogs: ethapi.FormatLogs(logs),
		Error:      formatError(err),
	}
}

// TraceBlockByHash processes the block by hash.
func (api *PrivateDebugAPI) TraceBlockByHash(hash common.Hash, config *vm.LogConfig) BlockTraceResult {
	// Fetch the block that we aim to reprocess
	block := api.eth.BlockChain().GetBlockByHash(hash)
	if block == nil {
		return BlockTraceResult{Error: fmt.Sprintf("block #%x not found", hash)}
	}

	validated, logs, err := api.traceBlock(block, config)
	return BlockTraceResult{
		Validated:  validated,
		StructLogs: ethapi.FormatLogs(logs),
		Error:      formatError(err),
	}
}

// traceBlock processes the given block but does not save the state.
func (api *PrivateDebugAPI) traceBlock(block *types.Block, logConfig *vm.LogConfig) (bool, []vm.StructLog, error) {
	// Validate and reprocess the block
	var (
		blockchain = api.eth.BlockChain()
		validator  = blockchain.Validator()
		processor  = blockchain.Processor()
	)

	structLogger := vm.NewStructLogger(logConfig)

	config := vm.Config{
		Debug:  true,
		Tracer: structLogger,
	}
	if err := api.eth.engine.VerifyHeader(blockchain, block.Header(), true); err != nil {
		return false, structLogger.StructLogs(), err
	}
	statedb, err := blockchain.StateAt(blockchain.GetBlock(block.ParentHash(), block.NumberU64()-1).Root())
	if err != nil {
		return false, structLogger.StructLogs(), err
	}

	receipts, _, usedGas, err := processor.Process(block, statedb, config)
	if err != nil {
		return false, structLogger.StructLogs(), err
	}
	if err := validator.ValidateState(block, blockchain.GetBlock(block.ParentHash(), block.NumberU64()-1), statedb, receipts, usedGas); err != nil {
		return false, structLogger.StructLogs(), err
	}
	return true, structLogger.StructLogs(), nil
}

// formatError formats a Go error into either an empty string or the data content
// of the error itself.
func formatError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type timeoutError struct{}

func (t *timeoutError) Error() string {
	return "Execution time exceeded"
}

// TraceTransaction returns the structured logs created during the execution of EVM
// and returns them as a JSON object.
func (api *PrivateDebugAPI) TraceTransaction(ctx context.Context, txHash common.Hash, config *TraceArgs) (interface{}, error) {

	log.Info("****TraceTransaction****")

	var tracer vm.Tracer
	if config != nil && config.Tracer != nil {
		timeout := defaultTraceTimeout
		if config.Timeout != nil {
			var err error
			if timeout, err = time.ParseDuration(*config.Timeout); err != nil {
				return nil, err
			}
		}

		var err error
		if tracer, err = ethapi.NewJavascriptTracer(*config.Tracer); err != nil {
			return nil, err
		}

		// Handle timeouts and RPC cancellations
		deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
		go func() {
			<-deadlineCtx.Done()
			tracer.(*ethapi.JavascriptTracer).Stop(&timeoutError{})
		}()
		defer cancel()
	} else if config == nil {
		tracer = vm.NewStructLogger(nil)
	} else {
		tracer = vm.NewStructLogger(config.LogConfig)
	}

	// Retrieve the tx from the chain and the containing block
	tx, blockHash, _, txIndex := core.GetTransaction(api.eth.ChainDb(), txHash)
	if tx == nil {
		return nil, fmt.Errorf("transaction %x not found", txHash)
	}
	msg, context, statedb, err := api.computeTxEnv(blockHash, int(txIndex))
	if err != nil {
		return nil, err
	}

	// Run the transaction with tracing enabled.
	log.Info("****TraceTransaction****")
	vmenv := vm.NewEVM(context, statedb, api.config, vm.Config{Debug: true, Tracer: tracer})
	ret, _, gas, failed, err := core.BinaryMessage(vmenv, msg, new(core.GasPool).AddGas(tx.Gas()), api.eth.Boker())
	if err != nil {
		return nil, fmt.Errorf("tracing failed: %v", err)
	}
	switch tracer := tracer.(type) {
	case *vm.StructLogger:
		return &ethapi.ExecutionResult{
			Gas:         gas,
			Failed:      failed,
			ReturnValue: fmt.Sprintf("%x", ret),
			StructLogs:  ethapi.FormatLogs(tracer.StructLogs()),
		}, nil
	case *ethapi.JavascriptTracer:
		return tracer.GetResult()
	default:
		panic(fmt.Sprintf("bad tracer type %T", tracer))
	}
}

// computeTxEnv returns the execution environment of a certain transaction.
func (api *PrivateDebugAPI) computeTxEnv(blockHash common.Hash, txIndex int) (core.Message, vm.Context, *state.StateDB, error) {

	log.Info("****computeTxEnv****")

	// Create the parent state.
	block := api.eth.BlockChain().GetBlockByHash(blockHash)
	if block == nil {
		return nil, vm.Context{}, nil, fmt.Errorf("block %x not found", blockHash)
	}
	parent := api.eth.BlockChain().GetBlock(block.ParentHash(), block.NumberU64()-1)
	if parent == nil {
		return nil, vm.Context{}, nil, fmt.Errorf("block parent %x not found", block.ParentHash())
	}
	statedb, err := api.eth.BlockChain().StateAt(parent.Root())
	if err != nil {
		return nil, vm.Context{}, nil, err
	}
	txs := block.Transactions()

	// Recompute transactions up to the target index.
	signer := types.MakeSigner(api.config, block.Number())
	for idx, tx := range txs {
		// Assemble the transaction call message
		msg, _ := tx.AsMessage(signer)
		context := core.NewEVMContext(msg, block.Header(), api.eth.BlockChain(), nil)
		if idx == txIndex {
			return msg, context, statedb, nil
		}

		vmenv := vm.NewEVM(context, statedb, api.config, vm.Config{})
		gp := new(core.GasPool).AddGas(tx.Gas())
		_, _, _, _, err := core.BinaryMessage(vmenv, msg, gp, api.eth.Boker())
		if err != nil {
			return nil, vm.Context{}, nil, fmt.Errorf("tx %x failed: %v", tx.Hash(), err)
		}
		statedb.DeleteSuicides()
	}
	return nil, vm.Context{}, nil, fmt.Errorf("tx index %d out of range for block %x", txIndex, blockHash)
}

// Preimage is a debug API function that returns the preimage for a sha3 hash, if known.
func (api *PrivateDebugAPI) Preimage(ctx context.Context, hash common.Hash) (hexutil.Bytes, error) {
	db := core.PreimageTable(api.eth.ChainDb())
	return db.Get(hash.Bytes())
}

// GetBadBLocks returns a list of the last 'bad blocks' that the client has seen on the network
// and returns them as a JSON list of block-hashes
func (api *PrivateDebugAPI) GetBadBlocks(ctx context.Context) ([]core.BadBlockArgs, error) {
	return api.eth.BlockChain().BadBlocks()
}

// StorageRangeResult is the result of a debug_storageRangeAt API call.
type StorageRangeResult struct {
	Storage storageMap   `json:"storage"`
	NextKey *common.Hash `json:"nextKey"` // nil if Storage includes the last key in the trie.
}

type storageMap map[common.Hash]storageEntry

type storageEntry struct {
	Key   *common.Hash `json:"key"`
	Value common.Hash  `json:"value"`
}

// StorageRangeAt returns the storage at the given block height and transaction index.
func (api *PrivateDebugAPI) StorageRangeAt(ctx context.Context, blockHash common.Hash, txIndex int, contractAddress common.Address, keyStart hexutil.Bytes, maxResult int) (StorageRangeResult, error) {
	_, _, statedb, err := api.computeTxEnv(blockHash, txIndex)
	if err != nil {
		return StorageRangeResult{}, err
	}
	st := statedb.StorageTrie(contractAddress)
	if st == nil {
		return StorageRangeResult{}, fmt.Errorf("account %x doesn't exist", contractAddress)
	}
	return storageRangeAt(st, keyStart, maxResult), nil
}

func storageRangeAt(st state.Trie, start []byte, maxResult int) StorageRangeResult {
	it := trie.NewIterator(st.NodeIterator(start))
	result := StorageRangeResult{Storage: storageMap{}}
	for i := 0; i < maxResult && it.Next(); i++ {
		e := storageEntry{Value: common.BytesToHash(it.Value)}
		if preimage := st.GetKey(it.Key); preimage != nil {
			preimage := common.BytesToHash(preimage)
			e.Key = &preimage
		}
		result.Storage[common.BytesToHash(it.Key)] = e
	}
	// Add the 'next key' so clients can continue downloading.
	if it.Next() {
		next := common.BytesToHash(it.Key)
		result.NextKey = &next
	}
	return result
}

// GetModifiedAccountsByumber returns all accounts that have changed between the
// two blocks specified. A change is defined as a difference in nonce, balance,
// code hash, or storage hash.
//
// With one parameter, returns the list of accounts modified in the specified block.
func (api *PrivateDebugAPI) GetModifiedAccountsByNumber(startNum uint64, endNum *uint64) ([]common.Address, error) {
	var startBlock, endBlock *types.Block

	startBlock = api.eth.blockchain.GetBlockByNumber(startNum)
	if startBlock == nil {
		return nil, fmt.Errorf("start block %x not found", startNum)
	}

	if endNum == nil {
		endBlock = startBlock
		startBlock = api.eth.blockchain.GetBlockByHash(startBlock.ParentHash())
		if startBlock == nil {
			return nil, fmt.Errorf("block %x has no parent", endBlock.Number())
		}
	} else {
		endBlock = api.eth.blockchain.GetBlockByNumber(*endNum)
		if endBlock == nil {
			return nil, fmt.Errorf("end block %d not found", *endNum)
		}
	}
	return api.getModifiedAccounts(startBlock, endBlock)
}

// GetModifiedAccountsByHash returns all accounts that have changed between the
// two blocks specified. A change is defined as a difference in nonce, balance,
// code hash, or storage hash.
//
// With one parameter, returns the list of accounts modified in the specified block.
func (api *PrivateDebugAPI) GetModifiedAccountsByHash(startHash common.Hash, endHash *common.Hash) ([]common.Address, error) {
	var startBlock, endBlock *types.Block
	startBlock = api.eth.blockchain.GetBlockByHash(startHash)
	if startBlock == nil {
		return nil, fmt.Errorf("start block %x not found", startHash)
	}

	if endHash == nil {
		endBlock = startBlock
		startBlock = api.eth.blockchain.GetBlockByHash(startBlock.ParentHash())
		if startBlock == nil {
			return nil, fmt.Errorf("block %x has no parent", endBlock.Number())
		}
	} else {
		endBlock = api.eth.blockchain.GetBlockByHash(*endHash)
		if endBlock == nil {
			return nil, fmt.Errorf("end block %x not found", *endHash)
		}
	}
	return api.getModifiedAccounts(startBlock, endBlock)
}

func (api *PrivateDebugAPI) getModifiedAccounts(startBlock, endBlock *types.Block) ([]common.Address, error) {
	if startBlock.Number().Uint64() >= endBlock.Number().Uint64() {
		return nil, fmt.Errorf("start block height (%d) must be less than end block height (%d)", startBlock.Number().Uint64(), endBlock.Number().Uint64())
	}

	oldTrie, err := trie.NewSecure(startBlock.Root(), api.eth.chainDb, 0)
	if err != nil {
		return nil, err
	}
	newTrie, err := trie.NewSecure(endBlock.Root(), api.eth.chainDb, 0)
	if err != nil {
		return nil, err
	}

	diff, _ := trie.NewDifferenceIterator(oldTrie.NodeIterator([]byte{}), newTrie.NodeIterator([]byte{}))
	iter := trie.NewIterator(diff)

	var dirty []common.Address
	for iter.Next() {
		key := newTrie.GetKey(iter.Key)
		if key == nil {
			return nil, fmt.Errorf("no preimage found for hash %x", iter.Key)
		}
		dirty = append(dirty, common.BytesToAddress(key))
	}
	return dirty, nil
}
