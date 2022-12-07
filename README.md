logtailer
=========

This is a service that is intended to run as a `DaemonSet` on Kubernetes. It
provides basic syslogging capabilities for services that don't support that
themselves. It watches logs in `/var/log/pods` and then tails the logs that
appear, if the pod has annotations that tell it to do so. It needs to run as a
service user that has access to query all pods in all namespaces in order to
read those annotations. Here's how it works:

1. A `PodTracker` watches `/var/log/pods` with the `DirListDiscoverer` to find
   any new pods

2. The `PodTracker` queries the `PodFilter` if it has a newly discovered pod,
   in order to determine if it should indeed track this pod.

3. In the event that it should track the pod, it starts up a `LogTailer` on
   each log file discovered by the `DirListDiscoverer`

4. The `LogTailer` is instantiated with a `LogOutput` that specifies what to
   do with log lines when they are retrieved. This is configured with a 
   `UDPSyslogger` output built around `logrus`.

5. When new log lines arrive, the `UDPSyslogger` strips off the Kubernetes-
   pecific preamble, and sends the remaining line wrapped in JSON with the
   specified additional fields present. This is the format we always send to
   Sumo Logic.

Configuration
-------------

This is configured with environment variables. They are all in `main.go` in
the `Config` struct at the top.

Services can be configured to have their logs tailed by `logtailer` by
adding an annotation to the template in the `Deployment`:

```
community.com/TailLogs=true
```

logtailer will discover those annotations and enable log tailing and
syslogging.

Running Locally for Testing
---------------------------

You can invoke this from the current directory using the test fixtures
to represent logfiles:

```
CACHE_FILE_PATH=./logtailer.json BASE_PATH=fixtures/pods DEBUG=true ./logtailer
```

It will fail to call Kubernetes for filtering and will proceed with a
`StubFilter` in place that always returns `true`.
