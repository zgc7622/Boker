// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package les implements the Light Ethereum Subprotocol.
package les

import (
	"fmt"
	"sync"
	"time"

	"github.com/Bokerchain/Boker/chain/accounts"
	"github.com/Bokerchain/Boker/chain/boker/api"
	"github.com/Bokerchain/Boker/chain/common"
	"github.com/Bokerchain/Boker/chain/common/hexutil"
	"github.com/Bokerchain/Boker/chain/consensus"
	"github.com/Bokerchain/Boker/chain/consensus/dpos"
	"github.com/Bokerchain/Boker/chain/core"
	"github.com/Bokerchain/Boker/chain/core/bloombits"
	"github.com/Bokerchain/Boker/chain/core/types"
	"github.com/Bokerchain/Boker/chain/eth"
	"github.com/Bokerchain/Boker/chain/eth/downloader"
	"github.com/Bokerchain/Boker/chain/eth/filters"
	"github.com/Bokerchain/Boker/chain/eth/gasprice"
	"github.com/Bokerchain/Boker/chain/ethdb"
	"github.com/Bokerchain/Boker/chain/event"
	"github.com/Bokerchain/Boker/chain/internal/ethapi"
	"github.com/Bokerchain/Boker/chain/light"
	"github.com/Bokerchain/Boker/chain/log"
	"github.com/Bokerchain/Boker/chain/node"
	"github.com/Bokerchain/Boker/chain/p2p"
	"github.com/Bokerchain/Boker/chain/p2p/discv5"
	"github.com/Bokerchain/Boker/chain/params"
	rpc "github.com/Bokerchain/Boker/chain/rpc"
)

type LightEthereum struct {
	odr                                        *LesOdr
	relay                                      *LesTxRelay
	chainConfig                                *params.ChainConfig
	shutdownChan                               chan bool
	peers                                      *peerSet
	txPool                                     *light.TxPool
	blockchain                                 *light.LightChain
	protocolManager                            *ProtocolManager
	serverPool                                 *serverPool
	reqDist                                    *requestDistributor
	retriever                                  *retrieveManager
	chainDb                                    ethdb.Database                 // Block chain database
	bloomRequests                              chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer, chtIndexer, bloomTrieIndexer *core.ChainIndexer
	ApiBackend                                 *LesApiBackend
	eventMux                                   *event.TypeMux
	engine                                     consensus.Engine
	accountManager                             *accounts.Manager
	networkId                                  uint64
	netRPCService                              *ethapi.PublicNetAPI
	wg                                         sync.WaitGroup
	password                                   string       //挖矿账号的密码
	boker                                      bokerapi.Api //播客链新增加的接口
}

func New(ctx *node.ServiceContext, config *eth.Config) (*LightEthereum, error) {
	chainDb, err := eth.CreateDB(ctx, config, "lightchaindata")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, isCompat := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !isCompat {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	peers := newPeerSet()
	quitSync := make(chan struct{})

	leth := &LightEthereum{
		chainConfig:    chainConfig,
		chainDb:        chainDb,
		eventMux:       ctx.EventMux,
		peers:          peers,
		reqDist:        newRequestDistributor(peers, quitSync),
		accountManager: ctx.AccountManager,
		//engine:           dpos.New(chainConfig.Dpos, chainDb),
		engine:           dpos.New(&params.DposConfig{}, chainDb),
		shutdownChan:     make(chan bool),
		networkId:        config.NetworkId,
		bloomRequests:    make(chan chan *bloombits.Retrieval),
		bloomIndexer:     eth.NewBloomIndexer(chainDb, light.BloomTrieFrequency),
		chtIndexer:       light.NewChtIndexer(chainDb, true),
		bloomTrieIndexer: light.NewBloomTrieIndexer(chainDb, true),
	}

	leth.relay = NewLesTxRelay(peers, leth.reqDist)
	leth.serverPool = newServerPool(chainDb, quitSync, &leth.wg)
	leth.retriever = newRetrieveManager(peers, leth.reqDist, leth.serverPool)
	leth.odr = NewLesOdr(chainDb, leth.chtIndexer, leth.bloomTrieIndexer, leth.bloomIndexer, leth.retriever)
	if leth.blockchain, err = light.NewLightChain(leth.odr, leth.chainConfig, leth.engine); err != nil {
		return nil, err
	}
	leth.bloomIndexer.Start(leth.blockchain)
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		leth.blockchain.SetHead(compat.RewindTo)
		core.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}

	leth.txPool = light.NewTxPool(leth.chainConfig, leth.blockchain, leth.relay)
	if leth.protocolManager, err = NewProtocolManager(leth.chainConfig, true, ClientProtocolVersions, config.NetworkId, leth.eventMux, leth.engine, leth.peers, leth.blockchain, nil, chainDb, leth.odr, leth.relay, quitSync, &leth.wg); err != nil {
		return nil, err
	}
	leth.ApiBackend = &LesApiBackend{leth, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.GasPrice
	}
	leth.ApiBackend.gpo = gasprice.NewOracle(leth.ApiBackend, gpoParams)
	return leth, nil
}

func lesTopic(genesisHash common.Hash, protocolVersion uint) discv5.Topic {
	var name string
	switch protocolVersion {
	case lpv1:
		name = "LES"
	case lpv2:
		name = "LES2"
	default:
		panic(nil)
	}
	return discv5.Topic(name + "@" + common.Bytes2Hex(genesisHash.Bytes()[0:8]))
}

type LightDummyAPI struct{}

// Coinbase is the address that mining rewards will be send to
func (s *LightDummyAPI) Coinbase() (common.Address, error) {
	return common.Address{}, fmt.Errorf("not supported")
}

// Hashrate returns the POW hashrate
func (s *LightDummyAPI) Hashrate() hexutil.Uint {
	return 0
}

// Mining returns an indication if this node is currently mining.
func (s *LightDummyAPI) Mining() bool {
	return false
}

// APIs returns the collection of RPC services the ethereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *LightEthereum) APIs() []rpc.API {
	return append(ethapi.GetAPIs(s.ApiBackend, nil), []rpc.API{
		{
			Namespace: "eth",
			Version:   "1.0",
			Service:   &LightDummyAPI{},
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux, s.Boker()),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, true, s.Boker()),
			Public:    true,
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *LightEthereum) Password() string {

	return s.password
}

func (s *LightEthereum) SetPassword(password string) {

	s.password = password
}

func (s *LightEthereum) SetCoinbase(coinbase common.Address) {
}

func (s *LightEthereum) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *LightEthereum) DecodeParams(code []byte) ([]byte, error) {

	return []byte(""), nil
}

func (s *LightEthereum) BlockChain() *light.LightChain      { return s.blockchain }
func (s *LightEthereum) TxPool() *light.TxPool              { return s.txPool }
func (s *LightEthereum) Engine() consensus.Engine           { return s.engine }
func (s *LightEthereum) LesVersion() int                    { return int(s.protocolManager.SubProtocols[0].Version) }
func (s *LightEthereum) Downloader() *downloader.Downloader { return s.protocolManager.downloader }
func (s *LightEthereum) EventMux() *event.TypeMux           { return s.eventMux }
func (s *LightEthereum) Boker() bokerapi.Api                { return s.boker }
func (s *LightEthereum) SetBoker(boker bokerapi.Api)        { s.boker = boker }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *LightEthereum) Protocols() []p2p.Protocol {
	return s.protocolManager.SubProtocols
}

// Start implements node.Service, starting all internal goroutines needed by the
// Ethereum protocol implementation.
func (s *LightEthereum) Start(srvr *p2p.Server) error {
	s.startBloomHandlers()
	log.Warn("Light client mode is an experimental feature")
	s.netRPCService = ethapi.NewPublicNetAPI(srvr, s.networkId)
	// search the topic belonging to the oldest supported protocol because
	// servers always advertise all supported protocols
	protocolVersion := ClientProtocolVersions[len(ClientProtocolVersions)-1]
	s.serverPool.start(srvr, lesTopic(s.blockchain.Genesis().Hash(), protocolVersion))
	s.protocolManager.Start()
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Ethereum protocol.
func (s *LightEthereum) Stop() error {
	s.odr.Stop()
	if s.bloomIndexer != nil {
		s.bloomIndexer.Close()
	}
	if s.chtIndexer != nil {
		s.chtIndexer.Close()
	}
	if s.bloomTrieIndexer != nil {
		s.bloomTrieIndexer.Close()
	}
	s.blockchain.Stop()
	s.protocolManager.Stop()
	s.txPool.Stop()

	s.eventMux.Stop()

	time.Sleep(time.Millisecond * 200)
	s.chainDb.Close()
	close(s.shutdownChan)

	return nil
}
