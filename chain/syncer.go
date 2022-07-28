package chain

import (
	"context"
	"fmt"

	"github.com/mitchellh/hashstructure"
	"github.com/statechannels/go-nitro-testground/peer"
	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/client/engine/chainservice"
	"github.com/statechannels/go-nitro/client/engine/store/safesync"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
	"github.com/testground/sdk-go/sync"
)

type shareableDeposit struct {
	ChannelId types.Destination
	Deposit   types.Funds
}

type shareableWithdrawAll struct {
	ChannelId   types.Destination
	SignedState state.SignedState
}

// ChainSyncer is responsible for keeping a local MockChain in sync with other clients
type ChainSyncer struct {
	client           sync.Client
	chain            chainservice.ChainService
	seenTransactions safesync.Map[bool]
	txListener       chan protocols.ChainTransaction
	depositTopic     *sync.Topic
	withdrawTopic    *sync.Topic
	ctx              context.Context
	me               peer.MyInfo
	quit             chan struct{}
}

// shareTransactions sends our transactions to other clients
func (c *ChainSyncer) shareTransactions() {

	for {
		select {
		case <-c.quit:
			return
		case raw := <-c.txListener:
			switch tx := raw.(type) {

			case protocols.DepositTransaction:
				c.client.MustPublish(c.ctx, c.depositTopic, shareableDeposit{ChannelId: tx.ChannelId(), Deposit: tx.Deposit})
			case protocols.WithdrawAllTransaction:
				c.client.MustPublish(c.ctx, c.depositTopic, shareableWithdrawAll{ChannelId: tx.ChannelId(), SignedState: tx.SignedState})
			}
		}
	}

}

// replayTransactions listens for transactions that occured on other client's chains and replays them on ours
func (c *ChainSyncer) replayTransactions() {

	deposits := make(chan shareableDeposit, 1_000_000)
	withdraws := make(chan shareableWithdrawAll, 1_000_000)
	fmt.Println()
	_ = c.client.MustSubscribe(c.ctx, c.depositTopic, deposits)
	_ = c.client.MustSubscribe(c.ctx, c.withdrawTopic, withdraws)

	for {
		select {
		case <-c.quit:
			return

		case t := <-deposits:
			tHash, err := hashstructure.Hash(t, &hashstructure.HashOptions{})
			if err != nil {
				panic(err)
			}
			if seenBefore, _ := c.seenTransactions.Load(fmt.Sprintf("%x", tHash)); !seenBefore {
				fmt.Printf("ChainSyncer: Replaying shared transaction %+v\n", t)

				_ = c.chain.SendTransaction(protocols.NewDepositTransaction(t.ChannelId, t.Deposit))
				c.seenTransactions.Store(fmt.Sprintf("%x", tHash), true)

			}

		case t := <-withdraws:
			fmt.Printf("%+v\n", t)
			tHash, err := hashstructure.Hash(t, &hashstructure.HashOptions{})
			if err != nil {
				panic(err)
			}
			if seenBefore, _ := c.seenTransactions.Load(fmt.Sprintf("%x", tHash)); !seenBefore {
				fmt.Printf("ChainSyncer: Replaying shared transaction %+v\n", t)
				_ = c.chain.SendTransaction(protocols.NewWithdrawAllTransaction(t.ChannelId, t.SignedState))
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
func NewChainSyncer(me peer.MyInfo, client sync.Client, ctx context.Context) *ChainSyncer {
	txListener := make(chan protocols.ChainTransaction, 1_000_000)

	chain := chainservice.NewMockChainWithTransactionListener(chainservice.NewMockChain(), me.Address, txListener)

	c := ChainSyncer{
		seenTransactions: safesync.Map[bool]{},
		client:           client,
		chain:            chain,
		depositTopic:     sync.NewTopic("deposit-transaction", shareableDeposit{}),
		withdrawTopic:    sync.NewTopic("withdraw-transaction", shareableWithdrawAll{}),
		me:               me,
		txListener:       txListener,
		quit:             make(chan struct{}),
		ctx:              ctx,
	}
	go c.replayTransactions()
	go c.shareTransactions()

	return &c
}

// ChainService returns the ChainService instance for this client
func (c *ChainSyncer) ChainService() chainservice.ChainService {
	return c.chain
}
