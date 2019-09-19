package mbpqs

import (
	"crypto/subtle"
	"fmt"
	"sync"
)

// SignatureSeqNo is the sequence number (index) of signatures and wotsKeys in channels and the root tree.
type SignatureSeqNo uint32

// RootSignature holds a signature on a channel by a rootTree leaf.
type RootSignature struct {
	ctx      *Context       // Defines the MBPQS instance which was used to create the Signature.
	seqNo    SignatureSeqNo // Sequence number of this signature so you know which index key to verify.
	drv      []byte         // Digest randomized value (r).
	wotsSig  []byte         // The WOTS signature over the channel root.
	authPath []byte         // The authentication path for this signature to the rootTree root node.
	rootHash []byte         // ChannelRoot which is signed.
}

// MsgSignature holds a signature on a message in a channel.
type MsgSignature struct {
	ctx        *Context       // Context defines the mbpqs instance which was used to create the signature.
	seqNo      SignatureSeqNo // Sequence number of this signature in the channel.
	drv        []byte         // Digest randomized value (r).
	wotsSig    []byte         // The WOTS signature over the channel message.
	authPath   []byte         // Autpath to the rootSignature.
	chainSeqNo uint32         // Sequence number of this signature in the used chain tree.
	chIdx      uint32         // In which channel the signature.
	layer      uint32         // From which chainTree layer the key comes.
	rootSig    *RootSignature // In case it is the first signature in a chain tree, it includes a pointer to the root signature.
}

// GrowSignature is a signature of the last OTS key in a chain tree over the next chain tree.
type GrowSignature struct {
	msgSig   *MsgSignature
	rootHash []byte
}

// Channel is a key channel within the MBPQS tree, are stacked chain trees with the same Tree address.
type Channel struct {
	idx        uint32         // The chIdx is the offset of the channel in the MBPQS tree.
	layers     uint32         // The amount of chain layers in the channel.
	chainSeqNo uint32         // The first signatureseqno available for signing in the channel (last chain).
	seqNo      SignatureSeqNo // The unique sequence number of the next available key.
	mux        sync.Mutex     // Used when mutual exclusion for the channel is required.
}

// PrivateKey is a MBPQS private key */
type PrivateKey struct {
	seqNo    SignatureSeqNo // The seqNo of the first unused signing key in the root Tree.
	Channels []*Channel     // Channels in the privatekey.
	/* n-byte skSeed is used to pseudorandomly generate wots channelkeys seeds.
	 * S in RFC8931, SK_1 and S in XMSS-T paper.
	 */
	skSeed []byte
	/* n-byte skPrf is used to randomize the message hash when signing.
	 * SK_PRF in RFC8931, SK_2 in XMSS-T paper.
	 */
	skPrf []byte
	/* n-byte pubSeed is used to randomize the hash to generate WOTS verification keys.
	 * SEED in RFC8931, SEED in XMSS-T paper.
	 */
	pubSeed []byte
	root    []byte            // n-byte root node of the root tree.
	ctx     *Context          // Context containing the MBPQS parameters.
	ph      precomputedHashes // Precomputed hashes from the pubSeed and skSeed.
	mux     sync.Mutex        // Used when mutual exclusion for the PrivateKey is required.
}

// PublicKey is a MBPQS public key.
type PublicKey struct {
	root []byte // n-byte root node of the root tree.
	/* n-byte pubSeed used to randomize the hash to generate WOTS verification keys.
	 * SEED in RFC8931, SEED in XMSS-T paper
	 */
	ph      precomputedHashes // Precomputed pubSeed hash.
	pubSeed []byte
	ctx     *Context // The context containing the algorithm definition for verifiers.
}

// InitParam returns a pointer to a Params struct with parameters initialized to given arguments.
func InitParam(n, rtH, chanH, ge uint32, w uint16) *Params {
	return &Params{
		n:     n,
		w:     w,
		rootH: rtH,
		chanH: chanH,
		ge:    ge,
	}
}

// GenerateKeyPair generates a new MBPQS keypair for given parameters.
func GenerateKeyPair(p *Params) (*PrivateKey, *PublicKey, error) {
	// Create new context including given parameters.
	ctx, err := newContext(*p)
	if err != nil {
		return nil, nil, err
	}

	// Set n-byte random seed values.
	skSeed, err := randomBytes(ctx.params.n)
	if err != nil {
		return nil, nil, err
	}
	skPrf, err := randomBytes(ctx.params.n)
	if err != nil {
		return nil, nil, err
	}
	pubSeed, err := randomBytes(ctx.params.n)
	if err != nil {
		return nil, nil, err
	}

	// Derive a keypair from the initialized Context.
	return ctx.deriveKeyPair(pubSeed, skSeed, skPrf)
}

// SignChannelRoot is used to sign the n-byte channel root hash with the PrivateKey
func (sk *PrivateKey) SignChannelRoot(chRt []byte) (*RootSignature, error) {
	// Create a new scratchpad to do the signing computations on to avoid memory allocations.
	pad := sk.ctx.newScratchPad()
	seqNo, err := sk.GetSeqNo()
	if err != nil {
		return nil, err
	}
	// Compute the digest randomized value (drv)
	drv := sk.ctx.prfUint64(pad, uint64(seqNo), sk.skPrf)
	// Hashed channelroot with H_msg
	hashChRt, err := sk.ctx.hashMessage(pad, chRt, drv, sk.root, uint64(seqNo))
	if err != nil {
		return nil, err
	}

	// Set otsAddr to calculate wotsSign over the message.
	var otsAddr address // All fields should be 0, that's why init is enough.
	// TODO: check address for OTS
	otsAddr.setOTS(uint32(seqNo)) // Except the OTS address which is seqNo = index.

	// Compute the root tree to build the authentication path
	rt := sk.ctx.genRootTree(pad, sk.ph)
	authPath := rt.AuthPath(uint32(seqNo))
	sig := RootSignature{
		ctx:      sk.ctx,
		seqNo:    seqNo,
		drv:      drv,
		wotsSig:  sk.ctx.wotsSign(pad, hashChRt, sk.pubSeed, sk.skSeed, otsAddr),
		authPath: authPath,
		rootHash: chRt,
	}
	return &sig, nil
}

// VerifyChannelRoot is used to verify the signature on the channel root.
func (pk *PublicKey) VerifyChannelRoot(rtSig *RootSignature, chRt []byte) (bool, error) {
	// Create a new scratchpad to do the verifiyng computations on.
	pad := pk.ctx.newScratchPad()
	hashChRt, err := pk.ctx.hashMessage(pad, chRt, rtSig.drv, pk.root, uint64(rtSig.seqNo))
	if err != nil {
		return false, err
	}

	// Derive the wotsPk from the signature.
	var otsAddr address // all fields are 0, like they are supposed to.
	otsAddr.setOTS(uint32(rtSig.seqNo))

	// Create the wotsPk on the scratchpad.
	wotsPk := pad.wotsBuf()
	pk.ctx.wotsPkFromSigInto(pad, rtSig.wotsSig, hashChRt, pk.ph, otsAddr, wotsPk)

	// Create the leaf from the wotsPk.
	var lTreeAddr address            // init with all fields 0.
	lTreeAddr.setType(lTreeAddrType) // Set address type.
	lTreeAddr.setLTree(uint32(rtSig.seqNo))
	curHash := pk.ctx.lTree(pad, wotsPk, pk.ph, lTreeAddr)

	// Now we use the authentication path to hash up to the root.
	var nodeAddr address
	var height uint32
	nodeAddr.setType(treeAddrType)

	index := uint32(rtSig.seqNo)
	for height = 1; height <= pk.ctx.params.rootH; height++ {
		nodeAddr.setTreeHeight(height - 1)
		nodeAddr.setTreeIndex(index >> 1)

		sibling := rtSig.authPath[(height-1)*pk.ctx.params.n : height*pk.ctx.params.n]

		var left, right []byte

		if index&1 == 0 {
			left = curHash
			right = sibling
		} else {
			left = sibling
			right = curHash
		}

		pk.ctx.hInto(pad, left, right, pk.ph, nodeAddr, curHash)
		index >>= 1
	}
	hashChRt = curHash

	if subtle.ConstantTimeCompare(hashChRt, pk.root) != 1 {
		return false, fmt.Errorf("invalid signature")
	}
	return true, nil
}

// GetSeqNo retrieves the current index of the first unusued channel signing key in the RootTree.
func (sk *PrivateKey) GetSeqNo() (SignatureSeqNo, error) {
	sk.mux.Lock()
	// Unlock the lock when the funtion is finished.
	defer sk.mux.Unlock()
	fmt.Println(sk.seqNo)
	// Check if there are still root keys left to sign channels with.
	if uint64(sk.seqNo) >= (1 << sk.ctx.params.rootH) {
		return 0, fmt.Errorf("no unused channel signing keys left")
	}
	sk.seqNo++
	return sk.seqNo - 1, nil
}

// GrowChannel sign the message 'msg' in the channel and checks for growth.
func (sk *PrivateKey) GrowChannel(chIdx uint32) (*GrowSignature, error) {
	ch := sk.getChannel(chIdx)
	if !(sk.ctx.deriveChainTreeHeight(ch.layers)-1 == uint32(ch.chainSeqNo)) {
		fmt.Printf("Tree height: %d\n", sk.ctx.deriveChainTreeHeight(ch.layers))
		fmt.Printf("ChainSeqNo: %d\n", uint32(ch.chainSeqNo))

		return nil, fmt.Errorf("last chainTree hasn't used its full capacity yet")
	}
	return sk.appendChainTree(chIdx)
}

// SignChannelMsg signs the message 'msg' in the channel with index chIdx.
// Be cautious: this
func (sk *PrivateKey) SignChannelMsg(chIdx uint32, msg []byte, lastOne bool) (*MsgSignature, error) {
	// Channels start from index 1.
	if chIdx == 0 {
		return nil, fmt.Errorf("channels start at index 1")
	}
	// Returns an error if the channel does not exist.
	if chIdx >= uint32(len(sk.Channels)+1) {
		return nil, fmt.Errorf("channel does not exist, please create it first")
	}
	ch := sk.getChannel(chIdx)
	// If the function call does not have the 'lastOne' flag, check if it is the last key
	// in the chain, so that it will not be used to sign a message instead of the next chain.
	if !lastOne && sk.ctx.deriveChainTreeHeight(ch.layers)-1 == uint32(ch.chainSeqNo) {
		return nil, fmt.Errorf("please grow the channel before signing new messages in it")
	}

	// Create scratchpad to avoid memory allocations.
	pad := sk.ctx.newScratchPad()
	// Retrieve and update chainSeqNo and channel seqNo
	chainSeqNo, seqNo := sk.ChannelSeqNos(chIdx)

	// 64-bit sigIdx, seed value for drv to avoid collisions with seqNo's in the root tree!
	// This value includes the channelID in the first 32 bits of the seed, and the seqNo in the last 32 bits.
	sigIdx := uint64(chIdx)<<32 + uint64(seqNo)

	// Compute drv (R) pseudorandomly from the seed.
	drv := sk.ctx.prfUint64(pad, sigIdx, sk.skPrf)

	chLayer := sk.getChannelLayer(chIdx)
	// Compute the chainTree.
	ct := sk.genChainTree(pad, chIdx, chLayer)

	// Set OTSaddr to calculate the Wots sig over the message.

	var otsAddr address
	otsAddr.setOTS(uint32(chainSeqNo))
	otsAddr.setLayer(chLayer)
	otsAddr.setTree(uint64(chIdx))

	// Select the authentication node in the tree.
	authPathNode := ct.AuthPath(uint32(chainSeqNo))

	hashMsg, err := sk.ctx.hashMessage(pad, msg, drv, sk.root, sigIdx)
	if err != nil {
		return nil, err
	}

	// These fields can only be set after check for required rootSignature is made.
	sig := &MsgSignature{
		ctx:        sk.ctx,
		chainSeqNo: chainSeqNo,
		seqNo:      seqNo,
		chIdx:      chIdx,
		layer:      chLayer,
		drv:        drv,
		wotsSig:    sk.ctx.wotsSign(pad, hashMsg, sk.pubSeed, sk.skSeed, otsAddr),
		authPath:   authPathNode,
	}

	return sig, nil
}

// Create a new channel, returns its index and the signature of its first chainTreeRoot.
func (sk *PrivateKey) createChannel() (uint32, *RootSignature, error) {
	// Determine the channelIndex.
	chIdx := uint32(len(sk.Channels) + 1)
	// Scratchpad to avoid computation allocations.
	pad := sk.ctx.newScratchPad()
	// Create a new channel, because it does not exist yet.
	ch := sk.deriveChannel(chIdx)
	// Appending the created channel to the channellist in the PK.
	sk.Channels = append(sk.Channels, ch)
	// Update the channel.
	ch.mux.Lock()
	ch.layers++
	ch.chainSeqNo = 0
	ch.mux.Unlock()

	// Create the first chainTree for the channel
	ct := sk.genChainTree(pad, chIdx, 1)
	// Get the root, and sign it.
	root := ct.getRootNode()
	// Sign the root.
	rtSig, err := sk.SignChannelRoot(root)
	if err != nil {
		return 0, nil, err
	}

	return chIdx, rtSig, nil
}

// VerifyChannelMsg return true if the signature/message pair is valid.
func (pk *PublicKey) VerifyChannelMsg(sig *MsgSignature, msg, authNode []byte) (bool, error) {
	pad := pk.ctx.newScratchPad()

	// 64-bit drvSeed value to avoid collisions with seqNo's in the root tree!
	// This value includes the channelID in the first 32 bits of the seed, and the seqNo in the last 32 bits.
	sigIdx := uint64(sig.chIdx)<<32 + uint64(sig.seqNo)

	// Hash the message with H_msg.
	hashMsg, err := pk.ctx.hashMessage(pad, msg, sig.drv, pk.root, sigIdx)
	if err != nil {
		return false, err
	}

	// Derive SubTreeAddr
	sta := SubTreeAddress{
		Layer: sig.layer,
		Tree:  uint64(sig.chIdx),
	}
	addr := sta.address()

	// Compute the wotsPk from the signature.
	var otsAddr address
	otsAddr.setSubTreeFrom(addr)
	otsAddr.setOTS(uint32(sig.chainSeqNo))
	wotsPk := pad.wotsBuf()
	pk.ctx.wotsPkFromSigInto(pad, sig.wotsSig, hashMsg, pk.ph, otsAddr, wotsPk)

	// Compute the leaf from the wotsPk.
	var lTreeAddr address
	lTreeAddr.setSubTreeFrom(addr)
	lTreeAddr.setType(lTreeAddrType)
	lTreeAddr.setLTree(uint32(sig.chainSeqNo))
	curHash := pk.ctx.lTree(pad, wotsPk, pk.ph, lTreeAddr)
	fmt.Printf("Leaf in verification, must be authpath for next: %x\n", curHash)
	// Now hash the leaf with the authentication path.
	var nodeAddr address
	nodeAddr.setSubTreeFrom(addr)
	nodeAddr.setType(treeAddrType)
	nodeAddr.setTreeHeight(pk.ctx.getNodeHeight(sig.layer, sig.chainSeqNo))
	nodeAddr.setTreeIndex(0)

	pk.ctx.hInto(pad, sig.authPath, curHash, pk.ph, nodeAddr, curHash)

	// Compare the computed value with the previous authentication path node.
	fmt.Printf("Current hash: %x\n", curHash)
	fmt.Printf("Authnode: %x\n", authNode)
	if subtle.ConstantTimeCompare(curHash, authNode) != 1 {
		return false, nil
	}
	return true, nil
}
