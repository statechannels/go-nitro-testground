# Go Nitro Testground Testplan
This implements a test plan for testground to run various go-nitro integration tests.

There are currently two test cases:
1. [create-virtual](./create-virtual.go): A scenario where multiple virtual channels are created with a configurable amount of peers and hubs.
2. [create-ledgers](./create-ledger.go): A scenario which creates directly funded ledger channels.

## Getting Started
### Installing
Install testground and build it
```sh
# This is a fork of the testground repo with support for M1 macs
git clone https://github.com/statechannels/testground.git
cd testground
make install

```

In a separate console start the daemon
```sh
testground daemon  # will start the daemon listening on localhost:8042 by default.
```

Register the go-nitro test plan with the testground
```sh
# imports the test plan from this repository into testground
testground plan import --from ../go-nitro-test-plan
```

### Running
This runs the `create-virtual` test-case with 3 nitro clients.
```sh
testground run s -p=go-nitro-test-plan -t=create-virtual -b=exec:go -r=local:exec -i 3
```