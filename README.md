# Intro

Bunny is a sidecar proxy (of sorts) for Kubernetes probes. By handling and transforming probes, we can both offer features that Kubernetes does not and improve those that already exist.

# Table of Contents
<!-- built with https://github.com/jonschlinkert/markdown-toc -->

<!-- toc -->

- [Acknowledgements](#acknowledgements)
- [Design](#design)
- [Status](#status)
- [Use Cases](#use-cases)
- [Alternatives](#alternatives)
- [Deployment Example](#deployment-example)
  * [Prerequisites For Deployment Example](#prerequisites-for-deployment-example)
  * [Creating the Kubernetes Deployment](#creating-the-kubernetes-deployment)
  * [Finding The Metrics And Traces](#finding-the-metrics-and-traces)
- [Configuration](#configuration)
  * [Pod Spec](#pod-spec)
  * [Environment Variables](#environment-variables)
  * [Config File](#config-file)
    + [egress](#egress)
      - [initialDelayMilliseconds](#initialdelaymilliseconds)
      - [periodMilliseconds](#periodmilliseconds)
      - [timeoutMilliseconds](#timeoutmilliseconds)
      - [probes](#probes)
        * [metrics](#metrics)
        * [httpGet](#httpget)
        * [grpc](#grpc)
        * [tcpSocket](#tcpsocket)
        * [exec](#exec)
    + [ingress](#ingress)
      - [httpServer](#httpserver)
        * [instantQuery](#instantquery)
        * [rangeQuery](#rangequery)
    + [signals](#signals)
    + [telemetry](#telemetry)
- [Known Issues and Bugs](#known-issues-and-bugs)
  * [failed to upload metrics: failed to send metrics to http://localhost:30001/otlp/v1/metrics: 400 Bad Request](#failed-to-upload-metrics-failed-to-send-metrics-to-httplocalhost30001otlpv1metrics-400-bad-request)
- [Building](#building)
  * [Prerequisites For Building](#prerequisites-for-building)
  * [Binaries](#binaries)
  * [Container Images](#container-images)
- [Development](#development)
  * [Prerequisites For Development](#prerequisites-for-development)
  * [Setting Up A Dev Environment](#setting-up-a-dev-environment)
- [Why Bunny and Bee?](#why-bunny-and-bee)

<!-- tocstop -->

# Acknowledgements

Bunny builds on the work of others, which we are grateful for:
* [OpenTelemetry](https://opentelemetry.io/) is a powerful framework for adding metrics and tracing to any codebase
* [Prometheus](https://prometheus.io/) but especially the contributors to the [usage page for TSDB](https://github.com/prometheus/prometheus/blob/main/tsdb/docs/usage.md)
* [otel-cli](https://github.com/equinix-labs/otel-cli) handled tracing exec probes for us. Just had to add it to the container image and set an environment variable to integrate it. Simple and powerful.
* [devslog](github.com/golang-cz/devslog) made reading logs easier and nicer. Send JSON to your centralized logging system but use this when reading logs locally.
* [Task](https://taskfile.dev/) saved us from having to use `make`. It's clear and easy to use.
* Grafana [Tempo](https://grafana.com/oss/tempo/) (for tracing) and [Mimir](https://grafana.com/oss/mimir/) (for metrics) - both were really easy to install via Helm and perfect for a local dev environment where we didn't have to worry about running up a bill.

# Design

Bunny is based on a couple of main ideas:
1. the existing Kubernetes probes are limited in how they perform checks
2. without either metrics or tracing on probes, it's difficult to tell how often they fail or under which conditions

Bunny solves this by:
1. Running the probes itself
2. Generating metrics and traces for those probes
3. Providing HTTP endpoints for Kubernetes to check against. These endpoints run Prometheus queries against the probe metrics to determine success or failure

Since Bunny runs the probes, we've improved on what Kubernetes already offers by:
* letting more or less probes run in every Pod - since probes just generate metrics and traces, they aren't tied to liveness, readiness, or startup
* running probes more often - by running a probe multiple times a second, more samples are collected, leading to a better representation of the health of the Pod
* having TCP probes support an [expect](https://en.wikipedia.org/wiki/Expect) like conversation test instead of just a port open test
* running exec probes inside Bunny's container, not the app container, which limits what they can access in the app container (increasing security)

The metrics that are generated are stored locally (allowing for fast queries) and can be scraped by any Prometheus compatible agent or pushed to an OpenTelemetry OTLP compatible metrics endpoint. Traces can also be pushed to an OTLP endpoint. Other non-OTLP endpoints can be easily added (provided that they're OpenTelemetry compatible).

Bunny can be reconfigured without recreating Pods. Since it watches for changes in its config file and applies them when it sees them, the probes for a Pod can be reconfigured without recreating the Pods. This unlocks a few use cases (see the Use Cases section below).

Security-wise, Bunny follows best practice:
* it runs as a non-root user
* it drops all capabilities
* runs in a read-only root filesystem
* the container image is based on scratch (so it doesn't have anything other than the bunny binary)

# Status

Please don't use Bunny in production. Or test it heavily if you do.

# Use Cases

* SRE tweaking readiness probes during regional off-hours to lower cost
    * Technically, this can be done already by updating the configuration of the Pod but that requires a rolling update of all the Pods in the Deployment (which could take minutes to hours). Bunny avoids this.
* Performance confidently tweaking probes to ensure that provisioned Pod capacity is utilized
* On-call engineers using tracing to more quickly understand readiness and liveness probe failure during an outage.
* Engineering leadership using traces to understand which downstream components most often increase the risk of failure leading to more informed decision making about where to focus engineering resources
* SRE using Prometheus' linear prediction to more proactively reject traffic and apply backpressure
* Engineering running probes more often (than Kubernetes allows), collecting more samples, and so getting a better representation of the failure rate of the service.

# Alternatives

Some of the functionality of Bunny could be recreated using different tools or patterns. In particular:
* basing a probe on a Prometheus query result could be implemented using an exec probe and `promtool`. This assumes that a Prometheus database already exists (potentially as another sidecar).
* adding traces to HTTP probes could be done by adding an NGINX proxy: https://opentelemetry.io/blog/2022/instrument-nginx/
* [otel-cli](https://github.com/equinix-labs/otel-cli) can already be used to trace programs run by exec probes. In fact, we include it in the container image for Bunny.

# Deployment Example

## Prerequisites For Deployment Example

1. Complete the "Prerequisites For Building" and "Prerequisites For Development" sections below
2. Set up a copies of Grafana, Mimir, and Tempo, by running `task install-grafana` (expect this to take 2-5 minutes to complete - there's lots of container images to pull and containers to start). You can access these at http://localhost:30000 with username `admin` and password `blarg`. Configuration for these can be found in `deploy/kubernetes/grafana`. These can be deleted with `task delete-grafana-all`.

## Creating the Kubernetes Deployment

To deploy Bee and Bunny to Kubernetes, run `task apply-bunny-deployment`. This will build the binaries and container images, then create the Deployment, Secret, and Services required. Files for this can be found in `deploy/kubernetes/bunny`. Note that the config for Bunny is in `deploy/kubernetes/bunny/bunny-secret.yaml` for the Kubernetes Deployment. This can be deleted with `task delete-bunny-deployment`.

## Finding The Metrics And Traces

1. Connect to Grafana using the link and credentials above. 
2. After logging in, on the top-left of the screen, to the left of the "Home" text, click on the the three lines, then select "Explore".
3. From the "Explore" view, you should see a drop-down list to the right of the "Outline" text. Click this to switch between "Mimir" (for metrics) and "Tempo" (for traces).

To see the metrics available:

1. Switch to Mimir
2. Click on the "Run query" button on the top-right
3. Click on the "Select metric" drop-down on the left
4. Click on the "Run query" button again
5. Click on the down arrow to the right of "Run query" to select an auto-update interval

For more info on how to use Mimir: https://grafana.com/docs/mimir/latest/

To see the traces available:

1. Switch to Tempo
2. Change the "Query type" to "Search" and click on the "Run query" button
3. Click on a Trace ID to get more details on it
5. Click on the down arrow to the right of "Run query" to select an auto-update interval

For more info on how to use Tempo: https://grafana.com/docs/tempo/latest/

# Configuration

To configure Bunny, a combination of environment variables and a config file are used. Also, since it runs as a sidecar with the same Pod as the app whose probes are being proxied, some additional Pod spec settings are required.

## Pod Spec

In the Pod's spec, you'll need to set the following:

* `shareProcessNamespace` needs to be true if you want to wait for your app's process to exit before Bunny shuts down. You'll want that if you want to ensure that Bunny keeps proxying probes until your app is shutdown. Also take a look at setting `signals.watchedProcessCommandLineRegEx` in Bunny's YAML file.
* `terminationGracePeriodSeconds` should be set to something high enough for your app and Bunny to cleanly shutdown (which includes sending all metrics and traces to their respective endpoints). By default, Kubernetes sets this to `30` but if your app needs more time, set to a value high enough for both your app and Bunny.

For Bunny's container spec in the Pod spec:

* `env` should be set based on the "Environment Variables" section below
* `livenessProbe`, `readinessProbe`, and `startupProbe` (if needed) need to be `httpGet` probes pointing to an endpoint defined in `ingress.health` in Bunny's config file (see the "Config File" section below).
* `ports` needs to be set to the port listed for the `ingress.httpServer.port` setting. See the "Config File" section below.
* `resources` should have a memory request set based on the memory requirements for Bunny. Also see the note on the `GOMEMLIMIT` and `GOGC` env vars in the "Environment Variables" section.
* `volumeMounts` is the recommended way to get Bunny's YAML file into the container. By using a volume mount of a ConfigMap or Secret, the content of the YAML file can be changed without recreating the Pod. Bunny will detect the change in content and automatically reload the file.

For your app's container spec in the Pod spec:

* `livenessProbe`, `readinessProbe`, and `startupProbe` should be unset and instead should be set in Bunny's YAML file, in the `egress` block. See the "Config File" section. 

## Environment Variables

The following env vars can be set for Bunny's container:

| Name                   | Default Value | Allowed Values | Purpose |
| :---                   | :---          | :---           | :---    |
| BUNNY_CONFIG_FILE_PATH | `/config/bunny.yaml` | a path to an existing YAML file | the path to the YAML file which will be used to further configure Bunny |
| LOG_HANDLER | unset | any string | when set to "TEXT", "text", "CONSOLE", or "console", easily readable logs are set to the terminal. Any other value (including have the env var unset) will result in JSON formatted logs |
| x_LOG_LEVEL | `info` | `INFO`, `info`, `DEBUG`, `debug`, `WARN`, `warn`, `ERROR`, `error` for the value. For the name of the env var, `x` should be replaced with the Go package name (for example `INGRESS`, `EGRESS`, `MAIN`, or `CONFIG`) | this configures the log level for each Go package in Bunny, allowing for more noisy logs to be filtered out |
| TZ | none | platform specific (see https://pkg.go.dev/time#Location) | this env var provided by Go and modifies the timezone of the logs. On Linux and macos, you likely want to set this to `UTC` |
| GOMEMLIMIT and GOGC | GOMEMLIMIT is set to result of `1024 * 1024 * 64` (which works out to 64 megs) and GOGC is set to `10` (which works out to 10 percent) | see https://tip.golang.org/doc/gc-guide | these env vars are also provided by Go. It is *strongly* recommended that both of these env vars be set based on tests performed in a staging environment. The value for `GOMEMLIMIT` should also be accounted for when setting the resources required to run this container in the Pod spec. |

Additional environment variables are also required for configuring the OpenTelemetry exporters set in the `telemetry.openTelemetry.exporters` section of the config file. For more details, see the "telemetry" sub-section of the "Config File" section below and https://opentelemetry.io/docs/instrumentation/go/exporters/. Note that both the `OTEL_METRIC_EXPORT_INTERVAL` and `OTEL_EXPORTER_OTLP_TIMEOUT` env vars should be set to values much lower than `terminationGracePeriodSeconds` for the Pod to ensure that Bunny exits quickly when Kubernetes deletes the Pod.

## Config File

<!-- TODO-LOW: write docs on using debug containers to access the filesystem of the container -->
<!-- this should include the `cd /proc/1/root/` trick -->

The YAML config file for Bunny contains most of the configuration for Bunny. It's used both when running Bunny outside of a container (mainly when developing) and in Kubernetes (where it could be stored in a Kubernetes Secret and volume mounted inside Bunny's container).

Each of the top level keys of the file map to a golang package for the project. They are:
* egress - which handles all the connections going out from Bunny
* ingress - which handles all the connection going into Bunny
* signals - which handles operating system signals (like SIGKILL when Kubernetes deletes a Pod)
* telemetry - which handles the configuration for Prometheus and OpenTelemetry

Details on each block follow:

### egress

#### initialDelayMilliseconds

How long to wait after the config file has been read before probes are run.

#### periodMilliseconds

How long to wait between running all the probes.

#### timeoutMilliseconds

How long before a probe times out. Can be longer than `periodMilliseconds`.

#### probes

A list of probes. Each probe has a `name`, a `metrics` block, and a probe action (either `httpGet`, `grpc`, `tcpSocket`, or `exec` - described further in their own sections below).

For example, here is an egress block with a `httpGet` probe action:

```yaml
egress:
  initialDelayMilliseconds: 0
  periodMilliseconds: 3000
  timeoutMilliseconds: 2000
  probes:
    - name: "alpha"
      httpGet:
        host: "localhost"
        httpHeaders:
          - name: "x-bunny-test-header-name"
            value: ["test-value"]
          - name: "x-bunny-test-header-name2"
            value: ["test-value2"]
        port: 2624
        path: "healthz"
      metrics:
        attempts:
          name: "egress_probe_alpha_attempts"
          enabled: true
          extraLabels:
            - name: "egress_probe_alpha_test_label_name"
              value: "egress_probe_alpha_test_label_value"
        responseTime:
          name: "egress_probe_alpha_response_time"
          enabled: true
          extraLabels: []
```

##### metrics

Each probe has three metrics that can be enabled:
* `attempts` - which counts the number of times that the probe has been attempted.
* `successes` - which counts the number of times that the probe has successfully completed. The criteria for success are different for each probe action but always include that the probe action completes before `timeoutMilliseconds`.
* `responseTime` - how long it took for a probe action to complete (in milliseconds).

Each metric block has the following keys:
* `name` - this is the name of the metric used by Prometheus. The value should be all lowercase with underscores separating words.
* `enabled` - a `true` or `false` value.
* `extraLabels` - (optional) a list of `key` and `value` pairs that is applied to this metric when scraped by a Prometheus compatible scraper or when pushed to an OTLP endpoint. Useful adding additional information to the metric (like the name of the Deployment, the region, or build version)

In the example above, we can see that the probe `alpha` only has `attempts` and `responseTime` metrics.

##### httpGet

The `httpGet` probe action is very similar to what Kubernetes already provides and has the following keys:

* `host` - the DNS name or IP address of the machine to connect to. Defaults to "localhost".
* `httpHeaders` - a list of `name` and `value` pairs where `value` is also a list of strings. These headers are sent with every HTTP GET request for the probe action.
* `port` - the port to connect to. Only integer values are valid.
* `path` - the path of the server to GET
* `scheme` - either "HTTP" or "HTTPS". Note that (like Kubernetes), if "HTTPS" is used, the certificate of the server connected to is *not* checked for validity.

##### grpc

The `grpc` probe action is also very similar to what Kubernetes provides and just has two keys:

* `port` - the port to connect to. Only integer values are valid.
* `service` - the name of the Health service to connect to.

##### tcpSocket

The `tcpSocket` probe action goes beyond what Kubernetes offers. Rather than just checking to see if the port can be opened, our version allows for a conversation to played out with regular expressions used to check the responses.

The keys for the probe action are:

* `host` - the DNS name or IP address of the machine to connect to. Defaults to "localhost".
* `port` - the port to connect to. Only integer values are valid.
* `expect` - (optional) a block with the following keys:
    * `send` - a block for sending text. It has the following keys: 
        * `text` - the text to send
        * `delimiter` - the text to end the message with
    * `receive` - a block describing the text that is expected to be received. It has the following keys:
        * `regex` - a regular expression to test the received text against
        * `delimiter` - the text which is expected to end the message

For example, in the `tcpSocket` probe action below, after connecting to `localhost:5248`, we send the string `hello` followed by a newline. We then wait and receive text until we receive a newline (based on the `delimiter` from the `receive` block). The regular expression is then checked against the received text. 

```yaml
egress:
  initialDelayMilliseconds: 0
  periodMilliseconds: 3000
  timeoutMilliseconds: 2000
  probes:
  - name: "delta"
    tcpSocket:
        host: "localhost"
        port: 5248
        expect:
        - send: 
            text: "hello"
            delimiter: "\n"
        - receive: 
            regex: "yellow"
            delimiter: "\n"
        - send:
            text: "how're ya now?"
            delimiter: "\n"
        - receive:
            regex: "can.* complain"
            delimiter: "\n"
```

##### exec

The `exec` probe action is fairly similar to what Kubernetes already offers. The differences are that:
1. it runs inside Bunny's container, not the app container, providing a degree of isolation from the app container
2. a trace for the `exec` probe is created by Bunny. The ID for this trace is passed to the program which is run via the `OTEL_CLI_FORCE_TRACE_ID` environment variable. This environment variable can then be consumed by `otel-cli`.

The `exec` probe has the following keys:
* `command` - a list of strings for the command to run and its arguments. The full path to the binary must be used and a shell is not automatically started.
* `env` - the environment variables to set for the command. As noted above, `OTEL_CLI_FORCE_TRACE_ID` is automatically set. It's a list of `name` and `value` pairs.

In the example that follows, we're using `otel-cli` to create a child span for the trace created by Bunny. Note that:
1. A bash shell is being created. For this example to work, the container image for Bunny would have to be changed to include this shell.
2. The `OTEL` environment variables are set here, despite Bunny having its own copy of the env vars. These are required for `otel-cli`.

```yaml
egress:
  initialDelayMilliseconds: 0
  periodMilliseconds: 3000
  timeoutMilliseconds: 2000
  probes:
  - name: "epsilon"
      exec:
        # the "\c" at the end is a different way of making echo not print a newline
        command: [ '/bin/bash', '-c', '/otel-cli exec --name saying-hello-to-bunny /bin/echo "Hi ${NICKNAME}!\c"' ]
        env:
          - name: "NICKNAME"
            value: "Bun Bun"
          - name: "OTEL_EXPORTER_OTLP_TIMEOUT"
            value: "1000" # 1000 = 1 second
          - name: "OTEL_EXPORTER_OTLP_PROTOCOL"
            value: "http/protobuf"
          # please don't do this - use certs if you can
          - name: "OTEL_EXPORTER_OTLP_INSECURE"
            value: "true"
          # this is the Grafana instance that we run locally
          # (see the "install-grafana" and related tasks in Taskfile.yml)
          # you'll likely want to change this to point to whatever OpenTelemetry service you're using
          - name: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
            value: "http://localhost:30002/v1/traces"
          - name: "OTEL_CLI_VERBOSE"
            value: "true"
```

### ingress

#### httpServer

Currently `ingress` only has one key. This may be expanded in the future. The keys for `httpServer` are:

* `port` - the port to connect to. Only integer values are valid.
* `readTimeoutMilliseconds`, `readHeaderTimeoutMilliseconds`, `writeTimeoutMilliseconds`, `idleTimeoutMilliseconds`, and `maxHeaderBytes` - the HTTP server provided by `ingress` is based on the one from the "net/http" package. See https://pkg.go.dev/net/http#Server for more info on these settings.
* `openTelemetryMetricsPath` - the path that should be used to scrape metrics from Bunny with a Prometheus compatible scraper if metrics are not being pushed to an OTLP metrics endpoint. See the `telemetry` block below for more details
* `prometheusMetricsPath` - the metrics path to use to scrape metrics from Prometheus' TSDB. Useful when debugging the checks in the `health` block below. When scraping metrics for storage in a centralized metrics store, you'll want to use the value from `openTelemetryMetricsPath` instead
* `health` - this block defines the health endpoints that Kubernetes will send HTTP probes to. The configuration for the HTTP probes that Kubernetes sends is in the Pod spec for Bunny (see the "Pod Spec" section above). For a complete example showing this, see the files in `deploy/kubernetes/bunny`. The `health` block contains the following keys:
    * `path` - the path for the health endpoint. In the example below, paths are based on their intended usage.
    * `metrics` - the metrics that should be generated for the queries defined in `instantQuery` or `rangeQuery`. Configured in the same way as the metrics for `egress`. See the `metrics` section above.
    * either `instantQuery` or `rangeQuery` - these define Prometheus PromQL queries which should be executed to determine if the the endpoint at `path` is successful or not. More details are these are provided in their own sections below.

An example `ingress` block:

```yaml
ingress:
  httpServer:
    port: 1312
    readTimeoutMilliseconds: 5000
    readHeaderTimeoutMilliseconds: 5000
    writeTimeoutMilliseconds: 10000
    idleTimeoutMilliseconds: 2000
    maxHeaderBytes: 10000
    openTelemetryMetricsPath: "otel-metrics"
    prometheusMetricsPath: "prom-metrics"
    health:
      - path: "healthz-liveness"
        instantQuery:
          timeout: "5s"
          relativeInstantTime: "-5s"
          query: "1.0 >= bool 0.2"
        metrics:
          attempts:
            name: "ingress_healthz_liveness_attempts"
            enabled: true
            extraLabels:
              - name: "ingress_healthz_liveness_attempts_label_name"
                value: "ingress_healthz_liveness_attempts_label_value"
          responseTime:
            name: "ingress_healthz_liveness_response_time"
            enabled: true
            extraLabels: []
      - path: "healthz-readiness"
        rangeQuery:
          timeout: "5s"
          relativeStartTime: "-5s"
          relativeEndTime: "0s"
          interval: "1s"
          query: "1.0 >= bool 0.2"
        metrics:
          attempts:
            name: "ingress_healthz_readiness_attempts"
            enabled: true
            extraLabels: []
          responseTime:
            name: "ingress_healthz_readiness_response_time"
            enabled: true
            extraLabels: []
```

##### instantQuery

An instant query is a Prometheus PromQL query for an instant in time. It includes the following keys:

* timeout - the timeout for the query as a string. For example "5s" would be 5 seconds.
* relativeInstantTime - the time to query for relative to the time the query is executed. For example "-5s" would mean 5 seconds in the past. Non-negative values don't make sense (the future hasn't happened yet).
* query - the PromQL query to send. The query's successful is based on what it returns:
    * scalar: if the value is equal to 1.0, the query is successful. Otherwise, not.
    * vector: if all values in the vector are equal to 1.0, the query is successful. Otherwise, not.
    * matrix: if all values in the matrix are equal to 1.0, the query is successful. Otherwise, not.
    * string: if the string is equal to "1" or "1.0", the query is successful. Otherwise, not.

##### rangeQuery

An instant query is a Prometheus PromQL query for a range of time. It includes the following keys:

* timeout - the timeout for the query as a string. For example "5s" would be 5 seconds.
* relativeStartTime: similar to `relativeInstantTime` for `instantQuery` but it defines the starting point of the time range.
* relativeEndTime: coupled with `relativeStartTime` to define the end of the time range. Must be more recent than `relativeStartTime`. For example, if we want to query over the last 5 seconds, `relativeStartTime` would be `-5s` and `relativeEndTime` would be `0s`.
* interval: the interval for which samples are taken and against which the query is performed against. For example, "1s" is every 1 second
* query - the PromQL query to send. The query's successful is based on what it returns:
    * scalar: if the value is equal to 1.0, the query is successful. Otherwise, not.
    * vector: if all values in the vector are equal to 1.0, the query is successful. Otherwise, not.
    * matrix: if all values in the matrix are equal to 1.0, the query is successful. Otherwise, not.
    * string: if the string is equal to "1" or "1.0", the query is successful. Otherwise, not.

### signals

The `signals` block contains a single key, `watchedProcessCommandLineRegEx`, that defines the regular expression to use when checking to see if any matching processes are running. This is useful to ensure that the app container has exited before Bunny shuts down.

For example, if we had a Python 3 based app, we might use something like the following, if we wanted to ensure that the wait completed regardless of which command line arguments were set on the app.

```yaml
signals:
  watchedProcessCommandLineRegEx: "/usr/bin/python3 /myapp/main.py .*"
```

### telemetry

This block handles the settings for OpenTelemetry (which we use for exporting metrics and traces) and Prometheus (which we use for querying metrics). It has the following keys:

* `openTelemetry`
    * `exporters` - the list of exporters to use. Valid values include `stdoutmetric`, `prometheus`, `otlpmetrichttp`, `otlpmetricgrpc`, `stdouttrace`, `otlptracehttp`, and `otlptracegrpc`. The exporters are configured through environment variables. See links for each of their docs at https://opentelemetry.io/docs/instrumentation/go/exporters/
* `prometheus`
    * `tsdbPath` - the path to the directory for Prometheus' time series database. Setting `tsdbPath` to an empty string results in a temp dir being created. With the recommended `securityContext` for Bunny, this will fail. Set a path here and ensure that a volume is mounted into Bunny's container (either an `emptyDir` or from a PersistentVolumeClaim)
    * `tsdbOptions` - settings which help manage the maximum size of the TSDB. These include `retentionDurationMilliseconds`, `minBlockDurationMilliseconds`, and `maxBlockDurationMilliseconds`. See https://pkg.go.dev/github.com/prometheus/prometheus@v0.48.1/tsdb#Options for a description of what these do. The other options are the defaults.
    * `promql`
      * `maxConcurrentQueries` - limit the number of concurrent queries against the Prometheus TSDB running inside Bunny. See https://pkg.go.dev/github.com/prometheus/prometheus@v0.48.1/promql#ActiveQueryTracker
      * `engineOptions` - each of the following options maps to their equivalent at https://pkg.go.dev/github.com/prometheus/prometheus@v0.48.1/promql#EngineOpts: `maxSamples`, `timeoutMilliseconds`, `lookbackDeltaMilliseconds`, and `noStepSubqueryIntervalMilliseconds`

An example `telemetry` block:

```yaml
telemetry:
  openTelemetry:
    exporters: 
      - "otlpmetrichttp"
      - "otlptracehttp"
  prometheus:
    tsdbPath: "/tsdb"
    tsdbOptions:
      retentionDurationMilliseconds: 3600000
      minBlockDurationMilliseconds: 300000
      maxBlockDurationMilliseconds: 900000
    promql:
      maxConcurrentQueries: 1000
      engineOptions:
        maxSamples: 50000000
        timeoutMilliseconds: 10000
        lookbackDeltaMilliseconds: 900000
        noStepSubqueryIntervalMilliseconds: 1000
```

# Known Issues and Bugs

## failed to upload metrics: failed to send metrics to http://localhost:30001/otlp/v1/metrics: 400 Bad Request

This looks like a bug in either OpenTelemetry or Grafana Mimir. From running Wireshark, it looks like otel tries to post to Mimir on that endpoint but doesn't send any metrics and Mimir responds by saying that the timestamp in the request was set to the unix epoch. So either otel needs to stop pushing when it has no metrics to send or Mimir needs to just accept pushes with no metrics.

# Building

## Prerequisites For Building

The build has a couple prereqs:
* [Homebrew](https://brew.sh/) is needed for the `brew install` commands below.
* A golang compiler that supports multiple architectures and operating systems. On a Mac, you get this with `brew install go`
* [Task](https://taskfile.dev/) need to be installed. Easily done with `brew install go-task` on a Mac.
* [Docker Desktop](https://www.docker.com/get-started/) - though technically we need access to some implementation of the `docker` command that we can run `docker build` with. You might be able to do this with other container build systems. The `Dockerfile` for Bunny and Bee are pretty simple.

## Binaries

In the root directory of the repo, run `task build-bunny`. This will build the binaries for each supported operating system and CPU combination. Likewise, for Bee, run `task build-bee`.

## Container Images

Same as above, just run `task build-bunny-docker-image` or `task build-bee-docker-image` instead. This will build the container images for each supported operating system and CPU combination, automatically building the binaries as well.

# Development

## Prerequisites For Development

The prereqs for dev include those for building, so install those first.

We also need:
* [helm](https://helm.sh/) - we just use this for installing Grafana, Mimir, and Tempo. Install it with `brew install helm`
* [kubectl](https://kubernetes.io/docs/reference/kubectl/) - Docker Desktop might provide this but the copy from Homebrew tends to be newer. Install it with `brew install kubernetes-cli`
* [ctlptl](https://github.com/tilt-dev/ctlptl) - this is used to configure Docker Desktop. Most of the time it's just used to start Docker Desktop before doing an image build. It's easily installed with `brew install tilt-dev/tap/ctlptl`
* A Kubernetes cluster. With `ctlptl`, it's setup for us in Docker Desktop. If you've already configured Docker Desktop's Kubernetes feature, `ctlptl` will just validate that it has been.

Aside from that, we use [vscode](https://code.visualstudio.com/) for editing with the following extensions:
* [Go](https://marketplace.visualstudio.com/items?itemName=golang.Go) - of course
* [Markdown Preview Mermaid Support](https://marketplace.visualstudio.com/items?itemName=bierner.markdown-mermaid)
* [Task](https://marketplace.visualstudio.com/items?itemName=task.vscode-task) - detects errors in `Taskfile.yml`
* [Todo Tree](https://marketplace.visualstudio.com/items?itemName=Gruntfuggly.todo-tree) - we should probably switch to GitHub Issues at some point but for now, this is lighter/faster/better
* [Code Spell Checker](https://marketplace.visualstudio.com/items?itemName=streetsidesoftware.code-spell-checker) - because we misspell things

## Setting Up A Dev Environment

Once the prereqs are installed, `git clone` a copy of the code and open `bunny.code-workspace`. That should configure vscode properly.

Start up Docker Desktop and make sure that Kubernetes is enabled for it. See https://docs.docker.com/desktop/kubernetes/ for how to do that.

To set up a copies of Grafana, Mimir, and Tempo, run `task install-grafana`. You can access these at http://localhost:30000 with username `admin` and password `blarg`. Configuration for these can be found in `deploy/kubernetes/grafana`.

Edit `deploy/local/bunny.yaml` for changing settings. It is pre-configured to send metrics and traces to the local instances of Mimir and Tempo (if you have those running) and to send probes to Bee.

Run `task run-bee` and `task run-bunny` to run copies of Bee and Bunny outside of a container. These will build the binaries if needed.

If you want to deploy Bee and Bunny to Kubernetes, run `task apply-bunny-deployment`. Files for this can be found in `deploy/kubernetes/bunny`. Note that the config for Bunny is in `deploy/kubernetes/bunny/bunny-secret.yaml` for the Kubernetes Deployment.

# Why Bunny and Bee?

They're nicknames for my cats.