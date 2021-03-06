# MBPQS #
Multi-Blockchain Post-Quantum Signatures (MBPQS) is a hash-based post-quantum secure cryptographic digtal signature scheme specifically designed for multi-chain blockchain systems.
The scheme is designed for private/consortium blockchain where orderers are predefined network peers, and sign blocks in segregated chains (channels).

## Parameters ##
The parameters for MBPQS are as following:

* **n**, chosen from {32,64}: security parameter in bytes. For `n=32`, `SHA-256` is used throughout the scheme, and for `n=64`, `SHA-512` is used.
* **w**, chosen from {4,16,256}: Winternitz parameter.
* **rootH**, integer < 20: height of the root tree, defines the maximum amount of channels which can be added (which is 2^rootH).
* **chanH**, integer < 2^32: height of the initial chain tree in a channel.
* **gf**, integer < (2^32-chanH): growing factor for subsequent chain trees relative to the previous chain tree in the channel. `gf=0` results in no relative growth of chain trees.

## Disclaimer ##
This code is meant to showcase the workings of MBPQS, cross-validation, and experimenting. 
Do NOT use this code any software deployment as cryptographic code requires careful consideration of the specific deployment environment.

## Installation ##
``` go get -u github.com/Breus/mbpqs ```

## References ##
The scheme design uses ideas from [XMSS-T](https://www.iacr.org/archive/pkc2016/96140179/96140179.pdf) to reach quantum-resistance, and the ChainTree structure from [BPQS](https://eprint.iacr.org/2018/658.pdf). 

The implementation of WOTS+ (including multi-target resistance from WOTS-T) and parts of the XMSS tree generation, including the corresponding unit tests, are taken from [Go-XMSS-MT](https://github.com/bwesterb/go-xmssmt) from [Bas Westerbaan](https://bas.westerbaan.name/). 
