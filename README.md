# Intro

Bunny is a sidecar proxy (of sorts) for Kubernetes probes. By handling and transforming probes, we can both offer features that Kubernetes does not and improve those that already exist.

# Status

Please don't use Bunny in production. Or test it heavily if you do.
<!-- TODO-MEDIUM: add an acknowledgements section (for otel-cli, OpenTelemetry, Prometheus, and the dev log formatting package that we use) -->
# Features

<!-- TODO-LOW: update the feature list -->
A (likely incomplete) feature list:

* config:
    * Reconfiguration without redeploying Pods via ionotify
* ingress:
* egress:
    * Performs HTTP GET, GRPC, TCP, and Exec probes
* logging:
    * JSON or console formatted logs
    * different log levels for each component of Bunny (e.g. ingress and egress related code can have different logging levels)
* telemetry:
    * support for Prometheus (for scraping metrics) and OTLP endpoints (for pushing metrics and traces)
    * extensible to other metrics and trace systems which OpenTelemetry supports
* signals:
    * waits for all processes with a given name to exit before shutting down (to ensure that all probes are handled until the app process exits)

# Use Cases

<!-- TODO-LOW: list more use cases -->
* SRE tweaking readiness probes during regional off-hours to lower cost
    * Technically, this can be done already by updating the configuration of the Pod but that requires a rolling update of all the Pods in the Deployment (which could take minutes to hours). Bunny avoids this.
* Performance confidently tweaking probes to ensure that provisioned Pod capacity is utilized
* On-call engineers using tracing to more quickly understand readiness and liveness probe failure during an outage.
* Engineering leadership using traces to understand which downstream components most often increase the risk of failure leading to more informed decision making about where to focus engineering resources
* SRE using Prometheus' linear prediction to more proactively reject traffic and apply backpressure
* Engineering running probes more often (than Kubernetes allows), collecting more samples, and so getting a better representation of the failure rate of the service.

# Alternatives

Some of the functionality of Bunny could be recreated using different tools or patterns. In particular:
* basing a probe on a Prometheus query result could be implemented using an exec probe and `promtool`
* adding traces to HTTP probes could be done by adding an NGINX proxy: https://opentelemetry.io/blog/2022/instrument-nginx/

# Deployment

Bunny runs as a sidecar container within the same Pod as your app's container. Configuration is mostly via a YAML file (with the remainder via environment variables). Bunny detects changes to the YAML file and automatically reconfigures itself.

# Example

<!-- TODO-LOW: show how to run a full example, with Bunny and Bee, inside of Docker Desktop, and with Grafana -->

# Configuration

To configure Bunny, a combination of environment variables and a config file are used. Also, since it runs as a sidecar with the same Pod as the app whose probes are being proxied, some additional Pod spec settings are required.

## Pod Spec

In the Pod's spec, you'll need to set the following:

* `shareProcessNamespace` needs to be true if you want to wait for your app's process to exit before Bunny shuts down. You'll want that if you want to ensure that Bunny keeps proxying probes until your app is shutdown. Also take a look at setting `signals.watchedProcessName` in Bunny's YAML file.
* `terminationGracePeriodSeconds` should be set to something high enough for your app and Bunny to cleanly shutdown (which includes sending all metrics and traces to their respective endpoints). By default, Kubernetes sets this to `30` but if your app needs more time, set to a value high enough for both your app and Bunny.
<!-- TODO-MEDIUM: what can we list for the securityContext? We should try for least privilege -->

For Bunny's container spec in the Pod spec:

* `env` should be set based on the "Environment Variables" section below
* `livenessProbe`, `readinessProbe`, and `startupProbe`
* `ports` needs to be set to the port listed for the `ingress.httpServer.port` setting. See the "Config File" section below.
* `resources` should have a memory request set based on the memory requirements for Bunny. Also see the note on the `GOMEMLIMIT` and `GOGC` env vars in the "Environment Variables" section.
* `volumeMounts` is the recommended way to get Bunny's YAML file into the container. By using a volume mount of a ConfigMap or Secret, the content of the YAML file can be changed without recreating the Pod. Bunny will detect the change in content and automatically reload the file.

For your app's container spec in the Pod spec:

* `livenessProbe`, `readinessProbe`, and `startupProbe` should be unset and instead should be set in Bunny's YAML file, in the `egress` block. See the "Config File" section.

## Environment Variables

The following env vars can be set:


| Name                   | Default Value | Allowed Values | Purpose |
| :---                   | :---          | :---           | :---    |
| BUNNY_CONFIG_FILE_PATH | `/config/bunny.yaml` | a path to an existing YAML file | the path to the YAML file which will be used to further configure Bunny |
| LOG_HANDLER | unset | any string | when set to "TEXT", "text", "CONSOLE", or "console", easily readable logs are set to the terminal. Any other value (including have the env var unset) will result in JSON formatted logs |
| x_LOG_LEVEL | `info` | `INFO`, `info`, `DEBUG`, `debug`, `WARN`, `warn`, `ERROR`, `error` for the value. For the name of the env var, `x` should be replaced with the Go package name (for example `INGRESS`, `EGRESS`, `MAIN`, or `CONFIG`) | this configures the log level for each Go package in Bunny, allowing for more noisy logs to be filtered out |
| TZ | none | platform specific (see https://pkg.go.dev/time#Location) | this env var provided by Go and modifies the timezone of the logs. On Linux and macos, you likely want to set this to `UTC` |
| GOMEMLIMIT and GOGC | GOMEMLIMIT is set to result of `1024 * 1024 * 64` (which works out to 64 megs) and GOGC is set to `10` (which works out to 10 percent) | see https://tip.golang.org/doc/gc-guide | these env vars are also provided by Go. It is *strongly* recommended that both of these env vars be set based on tests performed in a staging environment. The value for `GOMEMLIMIT` should also be accounted for when setting the resources required to run this container in the Pod spec. |

## Config File

<!-- TODO-LOW: write docs on the config file. Or think about adding comments to the example config file -->

# Known Issues and Bugs

## failed to upload metrics: failed to send metrics to http://localhost:30001/otlp/v1/metrics: 400 Bad Request

This looks like a bug in either OpenTelemetry or Grafana Mimir. From running Wireshark, it looks like otel tries to post to Mimir on that endpoint but doesn't send any metrics and Mimir responds by saying that the timestamp in the request was set to the unix epoch. So either otel needs to stop pushing when it has no metrics to send or Mimir needs to just accept pushes with no metrics.

# Building

<!-- TODO-LOW: write docs on building the binaries and Docker image -->

# Development

<!-- TODO-LOW: write docs on the dev process (including how to use the Taskfile to setup a local environment) -->