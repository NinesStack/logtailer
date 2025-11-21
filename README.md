logtailer
=========

**The problem this solves**

You have a Kubernetes cluster and you want to get all the logs for your
services. You don't necessarily want/need to ship all the logs, you only want
them for a selected list of pods. You want to prevent runaway logging by one
service from degrading logging for everyone else, and you don't want to blow
your logging budget every time someone mistakenly deploys with debug logging
enabled.

Downloading
-----------

The simplest way to get `logtailer` is from the Docker images pushed to
GitHub packages: https://github.com/NinesStack/logtailer/pkgs/container/logtailer

An example `manifest.yml` for deploying it is included in this source repo.

More Info
---------

This is an **opinionated** service that is intended to run as a `DaemonSet` on
Kubernetes. It provides basic syslogging capabilities for services that don't
support that themselves. It watches logs in `/var/log/pods` and then tails the
logs that appear, if the pod has annotations that tell it to do so. It needs to
run as a service user that has access to query all pods in all namespaces in
order to read those annotations.

The following decisions were made and are implemented here:

 * All logs will be sent over UDP. The format is not real syslog, but many
   log processing services handle this. For our use case, this works better
   than actual syslog.

 * Logs will be rate limited

 * The wrapper for log output will be JSON. If you are already encoding logs
   in JSON format, they will be wrapped in an outder JSON layer containing the
   metadata.
 
 * You are using `containerd` as the runtime on your K8s cluster.

How It Works
-------------

 1. A `PodTracker` watches `/var/log/pods` with the `DirListDiscoverer` to find
    any new pods
 
 2. The `PodTracker` queries the `PodFilter` if it has a newly discovered pod,
    in order to determine if it should indeed track this pod. The only
    `PodFilter` currently implemented makes a call to the Kubernetes API to look
    at the annotations on the pod. It expects a label called `ServiceName` to be
    used for filtering. This may or may not match `app` labels or similar that
    you already have.
 
 3. In the event that it should track the pod, it starts up a `LogTailer` on
    each log file discovered by the `DirListDiscoverer`
 
 4. The `LogTailer` is instantiated with a `LogOutput` that specifies what to do
    with log lines when they are retrieved. This is configured with a
    `UDPSyslogger` output built around `logrus`. This is an interface so it's
    entirely possible to implement different `LogOutput`s.
 
 5. Wrapped around the `UDPSyslogger` is a `RateLimitingLogger` that prevents
    logs from overruning the upstream. This is configurable with environment
    variables, like the rest of the service.
 
 6. When new log lines arrive, the `UDPSyslogger` strips off the Kubernetes-
    specific preamble, and sends the remaining line wrapped in JSON with the
    specified additional fields present. This is the format we always send to
    Sumo Logic.

Logging Format
--------------

The UDP-relayed output is of the following form:

```
{
   "Container" : "thecontainer",
   "Environment" : "dev",
   "Hostname" : "ip-10-1-11-123.us-west-2.compute.internal",
   "Level" : "info",
   "Payload" : "[2024-04-17T10:02:25 (agent) #1][Info] service: Received HTTP request",
   "PodName" : "default_service-74654768f-hqwwj_534d2d95-8eaa-4385-878e-c031bdc1c5c6",
   "ServiceName" : "your-service",
   "Timestamp" : "2024-04-17T10:02:25Z"
}
```

`Payload` contains the raw, original log line stripped of the
Kubernetes/containerd preamble.

Configuration
-------------

This is configured with environment variables. They are all in `main.go` in
the `Config` struct at the top.

Services can be configured to have their logs tailed by `logtailer` by
adding an annotation to the template in the `Deployment` (or similar
for other workloads):

```
community.com/TailLogs=true
```

`logtailer` will discover those annotations and enable log tailing and
syslogging.

Rate Limiter Reporting
----------------------

You can configure `logtailer` to report to New Relic Insights for reporting on
log rate limiting. This is of course quite limiting for those who are not New
Relic customers. A more open standard reporter will be forthcoming.

Enhanced Log Level Extraction
-----------------------------

You can configure `logtailer` to attempt to extract log levels with an enhanced regex
filter by setting the environment variable `ENABLE_REGEX_LOG_LEVEL_PARSING=true`.
By default this mode is disabled.

Running Locally for Testing
---------------------------

You can invoke this from the current directory using the test fixtures
to represent logfiles:

```
CACHE_FILE_PATH=./logtailer.json BASE_PATH=fixtures/pods DEBUG=true ./logtailer
```

It will fail to call Kubernetes for filtering and will proceed with a
`StubFilter` in place that always returns `true`.
