package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro-testground/config"
	"github.com/statechannels/go-nitro-testground/peer"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/testground/sdk-go/sync"

	"gonum.org/v1/gonum/graph/formats/gexf12"
)

type objectiveStatus string

const (
	Starting  objectiveStatus = "Starting"
	Completed objectiveStatus = "Completed"
)

// ObjectiveStatusInfo contains information about the status change of an objective
type ObjectiveStatusInfo struct {
	Id           protocols.ObjectiveId
	Time         time.Time
	Participants []common.Address
	ChannelId    string
	Status       objectiveStatus
}

// GraphRecorder is a utility for recording the state of the network in GEXF file format.
type GraphRecorder struct {
	me         peer.PeerInfo
	peers      []peer.PeerInfo
	config     config.RunConfig
	graph      *gexf12.Graph
	syncClient sync.Client
}

// Graph returns the GEXF graph struct
func (gr *GraphRecorder) Graph() *gexf12.Graph {
	return gr.graph
}

// NewGraphRecorder creates a new GraphRecorder
func NewGraphRecorder(me peer.PeerInfo, peers []peer.PeerInfo, config config.RunConfig, syncClient sync.Client) *GraphRecorder {
	var graph *gexf12.Graph
	if peer.IsGraphRecorder(me.Seq, config) {
		graph = &gexf12.Graph{}
		graph.TimeFormat = "dateTime"
		graph.Start = time.Now().Format(
			"2006-01-02T15:04:05-0700")
		graph.DefaultEdgeType = "directed"

		// Set up attributes on the node and edges for channels and participants
		graph.Attributes = []gexf12.Attributes{{
			Class: "node",
			Attributes: []gexf12.Attribute{
				{ID: "address", Title: "address", Type: "string"}, {ID: "role", Title: "role", Type: "string"}}},
			{
				Class: "edge",
				Attributes: []gexf12.Attribute{
					{ID: "channelId", Title: "ChannelId", Type: "string"}, {ID: "channelType", Title: "ChannelType", Type: "string"}}}}

		// Add nodes for each participant
		for _, p := range append(peers, me) {
			attValues := gexf12.AttValues{AttValues: []gexf12.AttValue{
				{For: "address", Value: p.Address.String()},
				{For: "role", Value: fmt.Sprintf("%d", p.Role)}}}

			node := gexf12.Node{ID: p.Address.String(),
				Label:     p.Address.String()[0:6],
				AttValues: &attValues,
				Start: time.Now().Format(
					"2006-01-02T15:04:05-0700")}

			graph.Nodes.Nodes = append(graph.Nodes.Nodes, node)
		}

	}

	gr := &GraphRecorder{
		me:         me,
		peers:      peers,
		config:     config,
		graph:      graph,
		syncClient: syncClient,
	}
	// Start listening for changes from other participants
	go gr.listenForShared(context.Background())

	return gr
}

// objectiveStatusChangedTopic returns the topic to share objective status changes on
func (gr *GraphRecorder) objectiveStatusChangedTopic() *sync.Topic {
	return sync.NewTopic("objective-status-change", ObjectiveStatusInfo{})
}

// listenForShared listens for objective status changes from other participants
func (gr *GraphRecorder) listenForShared(ctx context.Context) {
	if !peer.IsGraphRecorder(gr.me.Seq, gr.config) {
		return
	}
	started := make(chan ObjectiveStatusInfo)

	gr.syncClient.MustSubscribe(ctx, gr.objectiveStatusChangedTopic(), started)
	for {
		select {
		case s := <-started:
			gr.ObjectiveStatusUpdated(s)
		case <-ctx.Done():
			return
		}
	}
}

// ObjectiveStatusUpdated is called when an objective status changes
// It uses this information to build a graph of the network
func (gr *GraphRecorder) ObjectiveStatusUpdated(info ObjectiveStatusInfo) {
	if !peer.IsGraphRecorder(gr.me.Seq, gr.config) {
		gr.syncClient.MustPublish(context.Background(), gr.objectiveStatusChangedTopic(), info)
	}

	isCreateChannel := (strings.Contains(string(info.Id), "VirtualFund") || strings.Contains(string(info.Id), "DirectFund")) && info.Status == Starting
	isCloseChannel := (strings.Contains(string(info.Id), "VirtualDefund") || strings.Contains(string(info.Id), "DirectDefund")) && info.Status == Completed

	if isCreateChannel {
		channelType := "ledger"
		if strings.Contains(string(info.Id), "VirtualFund") {
			channelType = "virtual"
		}

		attValues := gexf12.AttValues{
			AttValues: []gexf12.AttValue{
				{For: "channelId", Value: info.ChannelId},
				{For: "channelType", Value: channelType}}}

		for i := 0; (i + 1) < len(info.Participants); i++ {
			gr.graph.Edges.Edges = append(gr.graph.Edges.Edges,
				gexf12.Edge{
					ID:     fmt.Sprintf("%s_%s", info.ChannelId, info.Participants[i]),
					Label:  string(info.ChannelId)[0:6],
					Source: info.Participants[i].String(),
					Target: info.Participants[i+1].String(),
					Start: info.Time.Format(
						"2006-01-02T15:04:05-0700"),
					AttValues: &attValues,
				},
			)
		}

	}
	if isCloseChannel {
		for i := 0; i < len(gr.graph.Edges.Edges); i++ {

			if strings.Contains(gr.graph.Edges.Edges[i].ID, info.ChannelId) {

				gr.graph.Edges.Edges[i].End = info.Time.Format(
					"2006-01-02T15:04:05-0700")
			}
		}
	}
}

