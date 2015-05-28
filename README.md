# logspout-redis-logstash
Logspout adapter for writing Docker container stdout/stderr logs to Redis in Logstash jsonevent layout.

See the example below for more information.


## Docker image available

Logspout including this adapter is available on Docker Hub. Pull it with:

```
$ docker pull rtoma/logspout-redis-logstash
```

## ELK integration

Try out logspout with redis-logstash adapter in a full ELK stack. A docker-compose.yml can be found in the example/ directory.

When logspout with adapter is running. Executing something like:

```
docker run --rm centos:7 echo hello from a container
```

Will result in a corresponding event in Elasticsearch. Below is a screenshot from Kibana4:

![](event-in-k4.png)
