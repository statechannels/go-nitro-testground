# Go Nitro Testground

This repository contains integration tests for the go-nitro client. It uses the [testground](https://docs.testground.ai/) test runner to run the tests.

There is currently only one test case: [virtual-payment](./tests/virtual-payment.go).

## Getting Started

### Prerequisites

Docker must be installed and the docker daemon must be running.

The tests submit transactions to and listen to events on a test blockchain such as [Hardhat network](https://hardhat.org/hardhat-network/docs/overview). You will need to install [dockerized hardhat](https://github.com/statechannels/hardhat-docker) or equivalent. Simply follow the instructions in the readme.


### Instructions

Install testground and build it:

```sh
git clone https://github.com/testground/testground.git
cd testground
make install

```

You will need to add an `.env.toml` file to your testground home directory. This file should declare the following configuration: 
    
```toml
[runners."local:docker"]
additional_hosts = ["chain"]
```

Next, start `hardhat-docker` (or your preferred alternative),

# NOTE: Double check no other programs are using that port 8545
```sh

docker run -it -d -p 8545:8545 --name chain hardhat
```

and then add the Hardhat docker container to the testground control network:

```sh
docker network connect testground-control chain
```



In a separate console start the daemon:

```sh
testground daemon  # will start the daemon listening on localhost:8042 by default.
```

Register the go-nitro test plan with the testground:

```sh
# imports the test plan from this repository into testground
testground plan import --from ../go-nitro-testground
```

Run the test:

```sh
 testground run s -p=go-nitro-testground -t=virtual-payment -b=exec:go -r=local:exec -tp=numOfHubs=1 -tp=numOfPayers=1 -tp=numOfPayees=2 -i=4 -tp=paymentTestDuration=10 -tp=concurrentPaymentJobs=2
```

**Note**: Testground uses [goproxy](https://goproxy.io/) to cache dependencies in a docker container. The first test run can be slow or timeout due to goproxy populating its cache. Subsequent runs should be much faster.

This requests a run of the `virtual-payment` test-case with:

- `-i=6` 6 separate instances, each with their own nitro client
- `-tp=numOfHubs=1` 1 instance will play the role of hub and act only as a intermediary
- `-tp=numOfPayers=1` 1 instance will play the role of payer and only send payments to payees
- `-tp=numOfPayees=2` 2 instances will play the role of payee and only accept payments
- `-tp=numOfPayeePayers=2` 2 instances will play the role of payeepayers and pay and accept payments.
- `-tp=paymentTestDuration=10` The payment test will run for 10 seconds.
- `-tp=concurrentPaymentJobs=2` Each non-hub will run two payment jobs.
- `-b=exec:go` compile locally on this machine
- `-r=local:exec` run the test locally on this machine

You should see console output in the console running `testground daemon`.

## Configuring grafana

Testground will automatically run a docker container for influxDb to record metrics and grafana to create dashboards against those metrics.

At the moment there is some manual configuration to connect grafana to the metrics database:


1. Go to the [local grafana instance](http://localhost:3000/datasources/new) and add a new **InfluxDb** datasource with the following properties:

   - **Url**: http://testground-influxdb:8086 
   - **Database**: testground

2. Import dashboards into [grafana](http://localhost:3000/dashboard/import) from the [dashboards directory](./dashboards/).
