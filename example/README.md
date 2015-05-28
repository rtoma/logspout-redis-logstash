# Example

This directory contains everything you need to try out the logspout-redis-logstash adapter.

This directory contains a docker-compose.yml that will allow you to quickly spin up linked & configured containers running these applications:

- logspout
- redis
- logstash
- elasticsearch
- kibana4


# Requirements

Have installed:

- docker
- docker-compose

On OSX:

- boot2docker


# Quickstart

Do a ```docker-compose up``` and all containers will start. Missing images will be pulled.

On OSX do ```open http://$(boot2docker ip):5601``` to open Kibana in your browser.

On other hosts: go to http://\<your-docker-host\>:5601.


# What, what, what?

Logspout captures stdout/stderr events from all running containers and pushes
them to a Redis list.

Logstash consumes events from the redis list, does some magic when events come
from Kibana and stores the events in Elasticsearch.

Kibana allows humans to search the events.
