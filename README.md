# Intro

Bunny is a sidecar proxy (of sorts) for Kubernetes probes. By handling and transforming probes, we can both offer features that Kubernetes does not and improve those that already exist.

<!-- TODO-MEDIUM: add a table of contents -->

# Acknowledgements

Bunny builds on the work of others, which we are grateful for:
* [OpenTelemetry](https://opentelemetry.io/) is a powerful framework for adding metrics and tracing to any codebase
* [Prometheus](https://prometheus.io/) but especially the contributors to the [usage page for TSDB](https://github.com/prometheus/prometheus/blob/main/tsdb/docs/usage.md)
* [otel-cli](https://github.com/equinix-labs/otel-cli) handled tracing exec probes for me. I just had to add it to the container image and set an environment variable to integrate it. Simple and powerful.
* [devslog](github.com/golang-cz/devslog) made reading logs easier and nicer. Send JSON to your centralized logging system but use this when reading logs locally.
* [Task](https://taskfile.dev/) saved me from having to use `make`. It's clear and easy to use.
* Grafana [Tempo](https://grafana.com/oss/tempo/) (for tracing) and [Mimir](https://grafana.com/oss/mimir/) (for metrics) - both were really easy to install via Helm and perfect for a local dev environment where I didn't have to worry about running up a bill.

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
<!-- TODO-LOW: finish describing this -->
* `livenessProbe`, `readinessProbe`, and `startupProbe` 
* `ports` needs to be set to the port listed for the `ingress.httpServer.port` setting. See the "Config File" section below.
* `resources` should have a memory request set based on the memory requirements for Bunny. Also see the note on the `GOMEMLIMIT` and `GOGC` env vars in the "Environment Variables" section.
* `volumeMounts` is the recommended way to get Bunny's YAML file into the container. By using a volume mount of a ConfigMap or Secret, the content of the YAML file can be changed without recreating the Pod. Bunny will detect the change in content and automatically reload the file.

For your app's container spec in the Pod spec:

* `livenessProbe`, `readinessProbe`, and `startupProbe` should be unset and instead should be set in Bunny's YAML file, in the `egress` block. See the "Config File" section.

## Environment Variables

The following env vars can be set for Bunny's container:

<!-- TODO-LOW: add more env vars to the docs -->

| Name                   | Default Value | Allowed Values | Purpose |
| :---                   | :---          | :---           | :---    |
| BUNNY_CONFIG_FILE_PATH | `/config/bunny.yaml` | a path to an existing YAML file | the path to the YAML file which will be used to further configure Bunny |
| LOG_HANDLER | unset | any string | when set to "TEXT", "text", "CONSOLE", or "console", easily readable logs are set to the terminal. Any other value (including have the env var unset) will result in JSON formatted logs |
| x_LOG_LEVEL | `info` | `INFO`, `info`, `DEBUG`, `debug`, `WARN`, `warn`, `ERROR`, `error` for the value. For the name of the env var, `x` should be replaced with the Go package name (for example `INGRESS`, `EGRESS`, `MAIN`, or `CONFIG`) | this configures the log level for each Go package in Bunny, allowing for more noisy logs to be filtered out |
| TZ | none | platform specific (see https://pkg.go.dev/time#Location) | this env var provided by Go and modifies the timezone of the logs. On Linux and macos, you likely want to set this to `UTC` |
| GOMEMLIMIT and GOGC | GOMEMLIMIT is set to result of `1024 * 1024 * 64` (which works out to 64 megs) and GOGC is set to `10` (which works out to 10 percent) | see https://tip.golang.org/doc/gc-guide | these env vars are also provided by Go. It is *strongly* recommended that both of these env vars be set based on tests performed in a staging environment. The value for `GOMEMLIMIT` should also be accounted for when setting the resources required to run this container in the Pod spec. |

## Config File

<!-- TODO-LOW: write docs on the config file. Or think about adding comments to the example config file -->

<!-- TODO-LOW: write docs on using debug containers to access the filesystem of the container -->
<!-- this should include the `cd /proc/1/root/` trick -->

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
* [Go](https://marketplace.visualstudio.com/items?itemName=golang.Go) - 
* [Markdown Preview Mermaid Support](https://marketplace.visualstudio.com/items?itemName=bierner.markdown-mermaid)
* [Task](https://marketplace.visualstudio.com/items?itemName=task.vscode-task)
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