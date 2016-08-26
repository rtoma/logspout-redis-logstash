# logspout-redis-logstash
[Logspout](https://github.com/gliderlabs/logspout) adapter for writing Docker container stdout/stderr logs to Redis in Logstash jsonevent layout.

Since v0.1.4 JSON input is supported, enabling you to add structure to your logs.


## Docker image available

Logspout including this adapter is available on [Docker Hub](https://registry.hub.docker.com/u/rtoma/logspout-redis-logstash/). Pull it with:

```
$ docker pull rtoma/logspout-redis-logstash
```

## How to use the Docker image

```
$ docker run -d --name logspout \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  rtoma/logspout-redis-logstash \
  redis://<your-redis-server>?key=bla&...
```

## Configuration

Some configuration can be passed via container environment keys.

Some can be passed via route options (e.g. `logspout redis://<host>?key=foo&password=secret`).

This table shows all configuration parameters:

| Parameter | Default | Environment key | Route option key |
|-----------|---------|-----------------|------------------|
| Enable debug, if set debug logging will be printed | disabled | DEBUG | debug |
| Redis password, if set this will force the adapter to execute a Redis AUTH command | none | REDIS_PASSWORD | password |
| Redis key, events will be pushed to this Redis list object | 'logspout' | REDIS_KEY | key |
| Redis database, if set the adapter will execute a Redis SELECT command | 0 | REDIS_DATABASE | database |
| Docker host, will add a docker.host=\<host\> field to the event, allowing you to add the hostname of your host, identifying where your container was running (think mesos) | none | REDIS\_DOCKER\_HOST | docker_host |
| Use Layout v0, what Logstash json format is used. With v0 JSON input support is disabled. | false (meaning we use v1) | REDIS\_USE\_V0\_LAYOUT | use_v0_layout |
| Logstash type, if set the event will get a @type property | none | REDIS\_LOGSTASH\_TYPE | logstash_type |
| If true, will replace all "." in container labels with "_". You need to set this if you are using Elasticsearch 2.x | false | DEDOT_LABELS | dedot_labels |
| Mute errors (to avoid error storm), disable by setting to other than 'true' | true | MUTE\_ERRORS | mute_errors |
| Redis connection timeout | 100 ms | CONNECT\_TIMEOUT | connect_timeout |
| Redis read timeout | 300 ms | READ\_TIMEOUT | read_timeout |
| Redis write timeout | 500 ms | WRITE\_TIMEOUT | write_timeout |

Note on timeouts: Logspout [stops tailing a container log](https://github.com/gliderlabs/logspout/blob/90302f046f740e3d77dda04f9a4387caed6f7f8d/router/pump.go#L288) if an adapter (like this one) takes longer than 1.0 second to process an event. That's why the sum of our default timeouts is a safe 900 ms.


## JSON input support

**Note:** this does not work when using the Logstash v0 layout.

Since v0.1.4 JSON input is supported, enabling you to add structure to your logs. Big words, but this is what it does.

Imagine your docker application to emit a log to stdout like:

```
{"message":"I was very busy","items_processed":42,"elapsed_time_ms":123}
```

Logspout-redis-logstash will recognize this log as a JSON string and will embed the fields into the JSON document. This is how the final JSON document looks like that will be send to Redis:

```
{
  "@timestamp": "2016-03-30T09:54:24.587277635Z",
  "host": "ef5fe78b6a8e",
  "message": "I was very busy",
  "docker": { ... },
  "event": {
    "elapsed_time_ms": 123,
    "items_processed": 42
  }
}
```

What just happened?

- the `message` field is set with the value from our input document.
- the document contains a `event` hash filled with all input fields (ex message)

### Logtypes

Using the `logtype` field in the input JSON doc, allows you to control the name of the field hash. This was added to give you some control over different logtypes you may want to implement.

Usecase: imagine you have an application that handles HTTP requests and wants to emit acceslog and applicationlog events. These events are different and you want to handle them differently.

We currently support two types:

- accesslog
- applog

Example input JSON to illustrate this "data wrangling" feature:

```
{"logtype":"applog","message":"something went bOOm!","level":"ERROR","line":42,"file":"source.go"}
```

Which results in this JSON doc for Redis:

```
{
  "@timestamp": "2016-03-30T10:03:00.016733514Z",
  "host": "16cecd099d78",
  "message": "something went bOOm!",
  "docker": { ... },
  "logtype": "applog",
  "applog": {
    "file": "source.go",
    "level": "ERROR",
    "line": 42
  }
}
```

See what happened?

- the `logtype` field is added and is set with the value from our input doc.
- the `event` hash is now called `applog`, because of the logtype

If you'd used `"logtype":"accesslog"` the JSON doc would have looked like:

```
{
  ... ,
  "logtype": "accesslog",
  "accesslog": {
    "file": "source.go",
    "level": "ERROR",
    "line": 42
  }
}
```



## Contribution

Want to add features? Feel welcome to submit a pull request!

If you are unable to code, feel free to create a issue describing your feature request or bug report. 


## Changelog

### 0.1.7-dev

- Added parameter for dedotting Docker labels (required for ES 2.x). Thanks to adepretis!

### 0.1.6

- Added parameters for Redis connection timeouts
- Error muting can now be disabled for troubleshooting
- Added tracing id in error messages
- Logging of successful rpush after error


### 0.1.5

- Added support for Docker labels. Thanks to teemupo!

### 0.1.4

- Added support for JSON input. See the paragraph for more information. Thanks to dickiedick62!

### 0.1.3

- Refactoring of configuration possiblities
- Introducing the 'Redis database' configuration parameter, allowing you to push events to a specific Redis database number.

### 0.1.2

- Bugfix: edge case when using a non-Hub image with port number (e.g. my.registry.com:443/my/image:tag)
- Added unit tests to test for regression

### 0.1.1

- The Redis adapter will now reconnect if Redis is unavailable or returns an error. Only 1 reconnect is attempted per event, so if it fails the event gets dropped. Thanks to @rogierlommers 
- You can now specify a @type field value. Only if specified this field will be added to the event document. Thanks to @dkhunt27

### 0.1.0

- initial version


## ELK integration

Try out logspout with redis-logstash adapter in a full ELK stack. A docker-compose.yml can be found in the example/ directory.

When logspout with adapter is running. Executing something like:

```
docker run --rm centos:7 echo hello from a container
```

Will result in a corresponding event in Elasticsearch. Below is a screenshot from Kibana4:

![](event-in-k4.png)


## Credits

Thanks to [Gliderlabs](https://github.com/gliderlabs) for creating Logspout!

Much thanks to all contributors.

For other credits see the header of the redis.go source file.
