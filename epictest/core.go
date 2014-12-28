package epictest

import (
	"io"

	context "github.com/jbenet/go-ipfs/Godeps/_workspace/src/code.google.com/p/go.net/context"
	datastore "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-datastore"
	sync "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-datastore/sync"

	blockstore "github.com/jbenet/go-ipfs/blocks/blockstore"
	blockservice "github.com/jbenet/go-ipfs/blockservice"
	exchange "github.com/jbenet/go-ipfs/exchange"
	bitswap "github.com/jbenet/go-ipfs/exchange/bitswap"
	bsnet "github.com/jbenet/go-ipfs/exchange/bitswap/network"
	importer "github.com/jbenet/go-ipfs/importer"
	chunk "github.com/jbenet/go-ipfs/importer/chunk"
	merkledag "github.com/jbenet/go-ipfs/merkledag"
	net "github.com/jbenet/go-ipfs/net"
	path "github.com/jbenet/go-ipfs/path"
	peer "github.com/jbenet/go-ipfs/peer"
	dht "github.com/jbenet/go-ipfs/routing/dht"
	uio "github.com/jbenet/go-ipfs/unixfs/io"
	util "github.com/jbenet/go-ipfs/util"
	"github.com/jbenet/go-ipfs/util/datastore2"
	delay "github.com/jbenet/go-ipfs/util/delay"
	eventlog "github.com/jbenet/go-ipfs/util/eventlog"
)

var log = eventlog.Logger("epictest")

// TODO merge with core.IpfsNode
type core struct {
	repo Repo

	blockService *blockservice.BlockService
	blockstore   blockstore.Blockstore
	dag          merkledag.DAGService
	id           peer.ID
}

func (c *core) ID() peer.ID {
	return c.repo.ID()
}

func (c *core) Bootstrap(ctx context.Context, p peer.ID) error {
	return c.repo.Bootstrap(ctx, p)
}

func (c *core) Cat(k util.Key) (io.Reader, error) {
	catterdag := c.dag
	nodeCatted, err := (&path.Resolver{catterdag}).ResolvePath(k.String())
	if err != nil {
		return nil, err
	}
	return uio.NewDagReader(nodeCatted, catterdag)
}

func (c *core) Add(r io.Reader) (util.Key, error) {
	nodeAdded, err := importer.BuildDagFromReader(
		r,
		c.dag,
		nil,
		chunk.DefaultSplitter,
	)
	if err != nil {
		return "", err
	}
	return nodeAdded.Key()
}

func makeCore(ctx context.Context, rf RepoFactory) (*core, error) {
	repo, err := rf(ctx)
	if err != nil {
		return nil, err
	}

	bss := &blockservice.BlockService{repo.Blockstore(), repo.Exchange()}
	dag := merkledag.NewDAGService(bss)
	// to make sure nothing is omitted, init each individual field and assign
	// all at once at the bottom.
	return &core{
		repo:         repo,
		blockService: bss,
		dag:          dag,
	}, nil
}

type RepoFactory func(ctx context.Context) (Repo, error)

type Repo interface {
	ID() peer.ID
	Blockstore() blockstore.Blockstore
	Exchange() exchange.Interface

	Bootstrap(ctx context.Context, peer peer.ID) error
}

type repo struct {
	// DHT, Exchange, Network,Datastore
	bitSwapNetwork bsnet.BitSwapNetwork
	blockstore     blockstore.Blockstore
	exchange       exchange.Interface
	datastore      datastore.ThreadSafeDatastore
	network        net.Network
	dht            *dht.IpfsDHT
	id             peer.ID
}

func (r *repo) ID() peer.ID {
	return r.id
}

func (c *repo) Bootstrap(ctx context.Context, p peer.ID) error {
	return c.dht.Connect(ctx, p)
}

func (r *repo) Datastore() datastore.ThreadSafeDatastore {
	return r.datastore
}

func (r *repo) Blockstore() blockstore.Blockstore {
	return r.blockstore
}

func (r *repo) Exchange() exchange.Interface {
	return r.exchange
}

func MocknetTestRepo(p peer.ID, n net.Network, conf Config) RepoFactory {
	return func(ctx context.Context) (Repo, error) {
		const kWriteCacheElems = 100
		const alwaysSendToPeer = true
		dsDelay := delay.Fixed(conf.BlockstoreLatency)
		ds := sync.MutexWrap(datastore2.WithDelay(datastore.NewMapDatastore(), dsDelay))

		log.Debugf("MocknetTestRepo: %s %s %s", p, n.LocalPeer(), n)
		dhtt := dht.NewDHT(ctx, p, n, ds)
		bsn := bsnet.NewFromIpfsNetwork(n, dhtt)
		bstore, err := blockstore.WriteCached(blockstore.NewBlockstore(ds), kWriteCacheElems)
		if err != nil {
			return nil, err
		}
		exch := bitswap.New(ctx, p, bsn, bstore, alwaysSendToPeer)
		return &repo{
			bitSwapNetwork: bsn,
			blockstore:     bstore,
			exchange:       exch,
			datastore:      ds,
			network:        n,
			dht:            dhtt,
			id:             p,
		}, nil
	}
}
