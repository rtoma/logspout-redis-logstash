# logspout-redis-logstash
Logspout adapter for writing Docker container stdout/stderr logs to Redis in Logstash jsonevent layout.

See the example below for more information.


## Docker container available

Logspout including this adapter is available on Docker Hub. Pull it with:

```
$ docker pull rtoma/logspout-redis-logstash
```

## Full example

Try out logspout with redis-logstash adapter in a full ELK stack. A docker-compose.yml can be found in the example/ directory.