package mbpqs

// Sequence number of signatures.
type SignatureSeqNo uint64

// PrivateKey is a MBPQS private key */
type PrivateKey struct {
	seqNo SignatureSeqNo // The seqNo of the first unused signing key.
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
}

// PublicKey is a MBPQS public key.
type PublicKey struct {
	height byte   // Height of the root tree.
	root   []byte // n-byte root node of the root tree.
	/* n-byte pubSeed used to randomize the hash to generate WOTS verification keys.
	 * SEED in RFC8931, SEED in XMSS-T paper
	 */
	pubSeed []byte
	ctx     *Context // The context containing the algorithm definition for verifiers.
}

// Return a pointer to a Params struct with parameters set to given arguments.
func setParam(n, w, rtH, chanH, d uint32) *Params {
	return &Params{
		n:         n,
		w:         w,
		rootH:     rtH,
		initChanH: chanH,
		d:         d,
	}
}

// Generate a new MBPQS keypair for given parameters.
func GenerateKeyPair(p Params) (*PrivateKey, *PublicKey, error) {
	// Create new context including given parameters.
	ctx, err := newContext(p)
	if err != nil {
		return nil, nil, err
	}

	// Set n-byte random seed values
	otsS, err := randomBytes(ctx.params.n)
	if err != nil {
		return nil, nil, err
	}
	pubS, err := randomBytes(ctx.params.n)
	if err != nil {
		return nil, nil, err
	}
	msgS, err := randomBytes(ctx.params.n)
	if err != nil {
		return nil, nil, err
	}

	return ctx.deriveKeyPair(otsS, pubS, msgS)
}
