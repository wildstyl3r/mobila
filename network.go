package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	libp2pwebrtc "github.com/libp2p/go-libp2p/p2p/transport/webrtc"
)

type Libp2pNode struct {
	Host  host.Host
	DHT   *dht.IpfsDHT
	Store *Store
}

func StartNode(ctx context.Context, store *Store) (*Libp2pNode, error) {
	privKey, err := store.LoadPrivateKey()

	if err == badger.ErrKeyNotFound {
		fmt.Println("New Peer ID...")
		privKey, _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, err
		}

		if err := store.SavePrivateKey(privKey); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, fmt.Errorf("error (wrong password?): %w", err)
	}

	h, _ := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.Transport(libp2pwebrtc.New),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/udp/0/webrtc-direct", "/ip4/0.0.0.0/tcp/0"),
		libp2p.NATPortMap(),
		libp2p.EnableRelay(),
		libp2p.EnableHolePunching())
	idht, _ := dht.New(ctx, h)

	node := &Libp2pNode{Host: h, DHT: idht, Store: store}

	go node.bootstrap(ctx)

	return node, nil
}

func (n *Libp2pNode) bootstrap(ctx context.Context) {

	peersFromDB, err := n.Store.LoadBootstrapPeers()
	if err != nil {
		panic(err)
	}
	for _, p := range peersFromDB {
		n.Host.Connect(ctx, p)
	}

	time.Sleep(5 * time.Second)
	if len(n.Host.Network().Peers()) == 0 {
		for _, addr := range dht.DefaultBootstrapPeers {
			pi, _ := peer.AddrInfoFromP2pAddr(addr)
			n.Host.Connect(ctx, *pi)
		}
	}
	n.DHT.Bootstrap(ctx)
}

func (n *Libp2pNode) Shutdown() {
	fmt.Println("Storing routing table")
	for _, pID := range n.DHT.RoutingTable().ListPeers() {
		info := n.Host.Peerstore().PeerInfo(pID)
		n.Store.SaveBootstrapPeer(info)
	}
	n.DHT.Close()
	n.Host.Close()
}
