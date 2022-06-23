package main

import (
	"context"
	"fmt"

	"github.com/mitchellh/hashstructure"
	"github.com/statechannels/go-nitro/client/engine/chainservice"
	"github.com/statechannels/go-nitro/client/engine/store/safesync"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/testground/sdk-go/sync"
)

// ChainSyncer is responsible for keeping a local MockChain in sync with other clients
type ChainSyncer struct {
	client           *sync.DefaultClient
	chain            *chainservice.MockChain
	seenTransactions safesync.Map[bool]
	txListener       chan protocols.ChainTransaction
	topic            *sync.Topic
	ctx              context.Context
	me               MyInfo
	quit             chan struct{}
}

// shareTransactions sends our transactions to other clients
func (c *ChainSyncer) shareTransactions() {

	for {
		select {
		case <-c.quit:
			return
		case tx := <-c.txListener:

			_, err := c.client.Publish(c.ctx, c.topic, &PeerTransaction{From: c.me.Address, Transaction: tx})
			if err != nil {
				panic(err)
			}
		}
	}

}

// replayTransactions listens for transactions that occured on other client's chains and replays them on ours
func (c *ChainSyncer) replayTransactions() {

	peerTransactions := make(chan *PeerTransaction)
	_, _ = c.client.Subscribe(c.ctx, c.topic, peerTransactions)
	for {
		select {
		case <-c.quit:
			return

		case t := <-peerTransactions:
			tHash, err := hashstructure.Hash(t, &hashstructure.HashOptions{})
			if err != nil {
				panic(err)
			}
			if seenBefore, _ := c.seenTransactions.Load(fmt.Sprintf("%x", tHash)); !seenBefore {
				fmt.Printf("Sending transaction %+v\n", t.Transaction)
				c.chain.SendTransaction(t.Transaction)
				c.seenTransactions.Store(fmt.Sprintf("%x", tHash), true)

			}
		}
	}

}

// Close stops the syncer
func (c *ChainSyncer) Close() {
	close(c.quit)
}

// NewChainSyncer creates a new chain and ChainSyncer and starts syncing with other client chains
func NewChainSyncer(me MyInfo, client *sync.DefaultClient, ctx context.Context) *ChainSyncer {
	txListener := make(chan protocols.ChainTransaction, 1_000_000)
	topic := sync.NewTopic("chain-transaction", &PeerTransaction{})
	chain := chainservice.NewMockChainWithTransactionListener(txListener)

	c := ChainSyncer{
		seenTransactions: safesync.Map[bool]{},
		client:           client,
		chain:            chain,
		topic:            topic,
		ctx:              ctx,
		me:               me,
		txListener:       txListener,
		quit:             make(chan struct{}),
	}
	go c.shareTransactions()
	go c.replayTransactions()
	return &c
}

// Mockchain returns the MockChain instance for this client
func (c *ChainSyncer) MockChain() *chainservice.MockChain {
	return c.chain
}
