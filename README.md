[![Coverage Status](https://coveralls.io/repos/github/robbert229/redis-template/badge.svg?branch=master)](https://coveralls.io/github/robbert229/redis-template?branch=master) [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT) [![Go Report Card](https://goreportcard.com/badge/github.com/robbert229/redis-template)](https://goreportcard.com/report/github.com/robbert229/redis-template)

# Introduction

redis-template is a small utility that performs a similar role as consul-template. The main difference is that
redis-template uses redis instead of consul. This does mean that this tool will not handle large loads as well as consul
since it is backed by redis, and uses its pubsub system to globally notify all listeners to reload, but this shouldn't
matter in most situations unless you have large scale traffic (which I do not).

## How To Use

Only a small subset of consul-template's functionality has been implemented.

### CLI Args

* you can read from a template, and write to a file.
* a command can be executed after reloading
* you can set a splay

```
./redis-template \
    -redis-addr localhost:6379 \
    -template "/app/config.json.tmpl:/app/config.json:echo eyo" \
    -splay 5s
```

### Template functions

* you can load a value from redis use key.
* you can attempt to load a value from redis, but use a default value if it is missing in redis.

```
    {{key "foo"}}
    {{keyOrDefault "foo" "bar"}}
```
