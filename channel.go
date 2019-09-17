package mbpqs

import (
	"fmt"
	"runtime"
	"sync"
)

/* Represents a height t chainTree of n-byte string nodes N[i,j] as:
 					N[t-1,0]
					/	 |
			  N(t-2,1)  N(t-2,1)
				/ |
			   (...)
			  /	  |
	      N(1,0) N(1,1)
		  /	  |
	 N(0,0)	 N(0,1)


	The buf array is structered as follows:
	[(0,0),(0,1),(1,0)(1,1),(...),(t-2,0)(t-2,1),(t-1,0)]
*/

type chainTree struct {
	height uint32
	n      uint32
	buf    []byte
}

// DeriveChannel creates a channel for chanelIdx.
func (sk *PrivateKey) deriveChannel(chIdx uint32) *Channel {
	return &Channel{
		idx:    chIdx,
		layers: 0,
		seqNo:  0,
	}
}

// Allocates a new ChainTree and returns a generated chaintree into the memory.
func (sk *PrivateKey) genChainTree(chIdx, chLayer uint32, pad scratchPad) chainTree {
	ct := newChainTree(sk.ctx.deriveChainTreeHeight(chLayer), sk.ctx.params.n)
	sk.genChainTreeInto(pad, chIdx, chLayer, ct)
	return ct
}

// Generates a chain tree into ct.
func (sk *PrivateKey) genChainTreeInto(pad scratchPad, chIdx, chLayer uint32, ct chainTree) {
	fmt.Println("Generating chainTree...")
	// Init addresses for OTS, LTree nodes, and Tree nodes.
	var otsAddr, lTreeAddr, nodeAddr address
	sta := SubTreeAddress{
		Layer: chLayer,
		Tree:  uint64(chIdx),
	}

	addr := sta.address()
	otsAddr.setSubTreeFrom(addr)
	otsAddr.setType(otsAddrType)
	lTreeAddr.setSubTreeFrom(addr)
	lTreeAddr.setType(lTreeAddrType)
	nodeAddr.setSubTreeFrom(addr)
	nodeAddr.setType(treeAddrType)

	// First, compute the leafs of the chain tree.
	var idx uint32
	if sk.ctx.threads == 1 {
		// No. leafs == height of the chain tree.
		for idx = 0; idx < ct.height; idx++ {
			lTreeAddr.setLTree(idx)
			otsAddr.setOTS(idx)

			copy(ct.leaf(idx), sk.ctx.genLeaf(pad, sk.ph, lTreeAddr, otsAddr))
		}
	} else {
		// The code in this branch does exactly the same as in the
		// branch above, but in parallel.
		wg := &sync.WaitGroup{}
		mux := &sync.Mutex{}
		var perBatch uint32 = 32
		threads := sk.ctx.threads
		if threads == 0 {
			threads = runtime.NumCPU()
		}
		wg.Add(threads)
		for i := 0; i < threads; i++ {
			go func(lTreeAddr, otsAddr address) {
				pad := sk.ctx.newScratchPad()
				var ourIdx uint32
				for {
					mux.Lock()
					ourIdx = idx
					idx += perBatch
					mux.Unlock()
					if ourIdx >= ct.height {
						break
					}
					ourEnd := ourIdx + perBatch
					if ourEnd > ct.height {
						ourEnd = ct.height
					}
					for ; ourIdx < ourEnd; ourIdx++ {
						lTreeAddr.setLTree(ourIdx)
						otsAddr.setOTS(ourIdx)
						copy(ct.leaf(ourIdx), sk.ctx.genLeaf(
							pad,
							sk.ph,
							lTreeAddr,
							otsAddr))
					}
				}
				wg.Done()
			}(lTreeAddr, otsAddr)
		}
		wg.Wait()
	}

	// Next, compute the internal nodes and the root node.
	var height uint32
	// Looping through all the layers of the chainTree.
	for height = 1; height < ct.height; height++ {
		// Set tree height of the computed node.
		nodeAddr.setTreeHeight(height - 1)
		// Internal nodes and root node have Treeindex 0.
		nodeAddr.setTreeIndex(0)
		sk.ctx.hInto(pad, ct.node(height-1, 0), ct.node(height-1, 1), sk.ph, nodeAddr, ct.node(height, 0))
	}
}

// Returns a slice of the leaf at given leaf index.
func (ct *chainTree) leaf(idx uint32) []byte {
	if idx == 0 {
		return ct.node(0, 0)
	}
	return ct.node((idx - 1), 1)
}

// Returns a slice of the node at given height and index idx in the chain tree.
func (ct *chainTree) node(height, idx uint32) []byte {
	ptr := ct.n * (2*height + idx)
	return ct.buf[ptr : ptr+ct.n]
}

// Gets the root node of the chain tree.
func (ct *chainTree) getRootNode() []byte {
	return ct.node(ct.height-1, 0)
}

// Allocates memory for a chain tree of n-byte strings with height-1.
func newChainTree(height, n uint32) chainTree {
	return chainTreeFromBuf(make([]byte, (2*height-1)*2), height, n)
}

// Makes a chain tree from a buffer.
func chainTreeFromBuf(buf []byte, height, n uint32) chainTree {
	return chainTree{
		height: height,
		n:      n,
		buf:    buf,
	}
}

// Returns the height of a chain tree at layer chainLayer.
func (ctx *Context) deriveChainTreeHeight(chainLayer uint32) uint32 {
	return ctx.params.chanH + ctx.params.ge*chainLayer
}

// GetChannelSeqNo retrieves the current index of the first signing key in the channel.
func (sk *PrivateKey) GetChannelSeqNo(chIdx uint32) SignatureSeqNo {
	ch := sk.Channels[chIdx]
	ch.mux.Lock()
	// Unlock the lock when the function is finished.
	defer ch.mux.Unlock()

	// TODO::::
	// For now, only one chain tree is possible
	if uint32(ch.seqNo) == sk.ctx.deriveChainTreeHeight(chIdx) {
		// TODO: make new chain
		return SignatureSeqNo(0)
	}
	ch.seqNo++
	return ch.seqNo - 1
}

func (ch *Channel) curLayer() uint32 {
	return ch.layers - 1
}