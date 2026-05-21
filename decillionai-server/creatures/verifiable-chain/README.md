# Verifiable Chain Extension 🧩

> Updated: **2026-04-14**

This extension is a container-VM toolkit for verifiable execution workflows on top of Caspar host APIs.

## Flow overview

1. **Execution node**
   - receives `onchainExecutionRequest`
   - verifies request signature rule
   - runs VM (`runVm`, commonly `elpify`)
   - publishes `onchainExecutionProofShared`
2. **Verifier nodes**
   - receive proof-shared message
   - verify via `elpifyProof`
   - publish `onchainVerificationVote`

## Extra chain capabilities

- PoS validator election (commit/reveal + stake-weighted randomness)
- Dynamic shard planning from machine load reports
- Chain driver actions (`upsertSubChain`, `rebalanceSubChains`)

## Runtime roles

- `VERIFIABLE_NODE_ROLE=executor|verifier`
- `VERIFIABLE_NODE_ID=<unique-id>`

## Build

```bash
docker build -t caspar-verifiable-chain sdk/creatures/verifiable-chain
```

## Security note ⚠️

The sample signature scheme is intentionally simplified for demonstrative use. Replace with production-grade signing and verification before deployment.
