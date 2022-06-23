# Go Nitro Testground Test-plan
This implements a test plan for testground to run various go-nitro integration tests.

There is currently only one test case: [create-ledger](./create-ledger.go): A scenario which creates directly funded ledger channels.


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
 testground run s -p=go-nitro-test-plan -t=virtual-payment -b=exec:go -r=local:exec -tp=numOfHubs=2 -i=5 -tp=paymentTestDuration=10 -tp=concurrentPaymentJobs=2
```
This requests a run of the `virtual-payment` test-case with:
- `-i=5` 5 instances with their own nitro client
- `-tp=numOfHubs=2` 2 instances will play the role of hub and act only as a intermediary 
- `-tp=paymentTestDuration=10` The payment test will run for 10 seconds.
- `-tp=concurrentPaymentJobs=2` Each non-hub will run two payment jobs.
- `-b=exec:go` compile locally on this machine
- `-r=local:exec` run the test locally on this machine


You should see console output in the console running `testground daemon`.

## Configuring grafana

Testground will automatically run a docker container for influxDb to record metrics and grafana to create dashboards against those metrics.

At the moment there is some manual configuration to connect grafana to the metrics database:

1. Find the IP of the influxDB container by running the following command:
    ```shell
    docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' testground-influxdb
    ```

2. Go to the [local grafana instance](http://localhost:3000/datasources/new) and add a new **InfluxDb** datasource with the following properties:

    - **Url**: http://192.18.0.6:8086 (where 192.18.0.6 is the IP from step 1)
    - **Database**: testground

3. Import dashboards into [grafana](http://localhost:3000/dashboard/import) from the [dashboards directory](./dashboards/).

Minor change