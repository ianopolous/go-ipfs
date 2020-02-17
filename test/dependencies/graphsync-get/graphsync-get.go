package main

import (
	"context"
	"log"
	"os"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-graphsync"
	gsimpl "github.com/ipfs/go-graphsync/impl"
	"github.com/ipfs/go-graphsync/ipldbridge"
	"github.com/ipfs/go-graphsync/network"
	"github.com/ipfs/go-graphsync/storeutil"
	"github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	ipldselector "github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
)

func newGraphsync(ctx context.Context, p2p host.Host, bs blockstore.Blockstore) (graphsync.GraphExchange, error) {
	network := network.NewFromLibp2pHost(p2p)
	ipldBridge := ipldbridge.NewIPLDBridge()
	return gsimpl.New(ctx,
		network, ipldBridge,
		storeutil.LoaderForBlockstore(bs),
		storeutil.StorerForBlockstore(bs),
	), nil
}

var selectAll ipld.Node = func() ipld.Node {
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())
	return ssb.ExploreRecursive(
		ipldselector.RecursionLimitDepth(100), // default max
		ssb.ExploreAll(ssb.ExploreRecursiveEdge()),
	).Node()
}()

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("expected a multiaddr and a CID, got %d args", len(os.Args)-1)
	}
	addr, err := multiaddr.NewMultiaddr(os.Args[1])
	if err != nil {
		log.Fatalf("failed to multiaddr '%q': %s", os.Args[1], err)
	}
	ai, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		log.Fatal(err)
	}

	target, err := cid.Decode(os.Args[2])
	if err != nil {
		log.Fatalf("failed to decode CID '%q': %s", os.Args[2], err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p2p, err := libp2p.New(ctx, libp2p.NoListenAddrs)
	if err != nil {
		log.Fatal(err)
	}
	err = p2p.Connect(ctx, *ai)
	if err != nil {
		log.Fatal(err)
	}

	bs := blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore()))
	gs, err := newGraphsync(ctx, p2p, bs)
	if err != nil {
		log.Fatal("failed to start", err)
	}
	resps, errs := gs.Request(ctx, ai.ID, cidlink.Link{Cid: target}, selectAll)
	for {
		select {
		case <-ctx.Done():
			log.Fatal(ctx)
			return
		case _, ok := <-resps:
			if !ok {
				resps = nil
			}
		case err, ok := <-errs:
			if !ok {
				// done.
				return
			}
			if err != nil {
				log.Fatalf("got an unexpected error: %s", err)
			}
		}
	}
}
