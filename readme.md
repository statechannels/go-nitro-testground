# Go Nitro Testground Test-plan
This implements a test plan for testground to run various go-nitro integration tests.

There are currently two test cases:
1. [create-virtual](./create-virtual.go): A scenario where multiple virtual channels are created with a configurable amount of peers and hubs.
2. [create-ledger](./create-ledger.go): A scenario which creates directly funded ledger channels.

FYI: The `go-nitro-test-plan` module depends on a specific [branch](https://github.com/statechannels/go-nitro/tree/only-client-close) of `go-nitro`.


## Getting Started
**Prerequisite:** Docker must be install and the docker daemon must be running.

Install testground and build it:
```sh
# This is a fork of the testground repo with support for M1 macs
git clone https://github.com/statechannels/testground.git
cd testground
make install

```

In a separate console start the daemon:
```sh
testground daemon  # will start the daemon listening on localhost:8042 by default.
```

Register the go-nitro test plan with the testground:
```sh
# imports the test plan from this repository into testground
testground plan import --from ../go-nitro-test-plan
```
Run the test:
```sh
testground run s -p=go-nitro-test-plan -t=create-virtual -b=exec:go -r=local:exec -tp=numOfChannels=4 -tp=numOfHubs=2 -i=5
```
This requests a run of the `create-virtual` test-case with:
- `-i=5` 5 instances with their own nitro client
- `-tp=numOfHubs=2` 2 instances will play the role of hub and act only as a intermediary 
- `-tp=numOfChannels=4` each non-hub instance will create 4 virtual channels with randomly selected hub and peer
- `-b=exec:go` compile locally on this machine
- `-r=local:exec` run the test locally on this machine

You should see console output in the console running `testground daemon`.