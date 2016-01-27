# logspout-redis-logstash
[Logspout](https://github.com/gliderlabs/logspout) adapter for writing Docker container stdout/stderr logs to Redis in Logstash jsonevent layout.

See the example below for more information.


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
| Use Layout v0, what Logstash json format is used | false (meaning we use v1) | REDIS\_USE\_V0\_LAYOUT | use_v0_layout |
| Logstash type, if set the event will get a @type property | none | REDIS\_LOGSTASH\_TYPE | logstash_type |


## Contribution

Want to add features? Feel welcome to submit a pull request!

If you are unable to code, feel free to create a issue describing your feature request or bug report. 

## Changelog

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

For other credits see the header of the redis.go source file.
