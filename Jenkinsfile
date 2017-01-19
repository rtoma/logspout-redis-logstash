#!groovy
@Library('olisipo@2.1') _

wrappedNode {
  // docker config
  def docker = tool('System Docker') + '/docker'
  def dockerRegistry = 'artifacts.ath.bskyb.com:5001'
  def imageVersion = '1.0'
  def imageName = "${dockerRegistry}/olisipo/logspout-redis-logstash:${imageVersion}"

  // logspout and logspout-redis-logstash config
  def logspoutRedisLogstashSpec = '65e22f7'
  def logspoutSpec = '6c88eec'

  stage ('Prepare') {
      checkout scm
  }

  stage ('Build Docker image') {
    sh(script: "./build.sh -c -v ${imageVersion} ${logspoutRedisLogstashSpec} ${logspoutSpec}")
  }

  stage ('Push Docker image') {
      execDocker "push ${imageName}", docker
  }

  stage ('Cleanup Docker image') {
      execDocker "rmi ${imageName}", docker
  }
}
