# Service Simulator

This is a basic program that can simulate a set of nodes in a service. This is somewhat crude but does the majority of the job. This is a simulation so no actual client calls are occurring but instead we are faking the metrics.

Different actions can be taken through API calls to cause a node to fail, remove a node from rotation, fix a node and add it back into rotation and run through a deployment.

The service comes packaged with Prometheus and Grafana that are setup by Docker Compose.

## Building

The service simulator program is written in Go so you must have a working Go toolchain avaiable.

To build, simply run:

```bash
go build
```

This will build a binary called `svc-sim` that you can run.

You will need to install Docker and Docker Compose on your machine to get Prometheus and Grafana running. To run Prometheus and Grafana, do the following:

```bash
cd prometheus-grafana
docker-compose up -d
```

You should now be able to access Grafana at http://127.0.0.1:3000 and Prometheus at http://127.0.0.1:9090. Prometheus is setup to scrape the `svc-sim` service on its default port.

## Controlling

We can control what the service does by hitting some endpoints.

* `/fail-node?node=##` will cause a specified node to begin produce 500 error responses. If no node is specified as a query parameter, we will randomly select a node to fail.
* `/remove-node?node=##` will remove a node from the rotation. This will cause the node to stop producing metrics which will look like it has been removed.
* `/fix-node?node=##` will remove errors from a bad node and add it back into the rotation.
* `/deploy` will simulate a deployment where a node is taken out of rotation, updated and added back into the rotation.

For all of these operations we rebalance the number of expected requests per second (RPS). If a node is removed, the other nodes will begin taking more requests and when a node is added the requests will rebalance between the nodes. Some jitter is added to the requests so you will stay close to the expected number of requests but not exactly.