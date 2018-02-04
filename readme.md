# Introduction

redis-template is a small utility that performs a similar role as consul-template. The main difference is that
redis-template uses redis instead of consul. This does mean that this tool will not handle large loads as well as consul
since it is backed by redis, and uses its pubsub system to globally notify all listeners to reload, but this shouldn't
matter in most situations unless you have large scale traffic (which I do not).

## How To Use

Only a small subset of consul-template's functionality has been implemented.

* you can read from a template, and write to a file.
* a command can be executed after reloading
* you can set a splay

```
./redis-template \
    -redis-addr localhost:6379 \
    -template "/app/config.json.tmpl:/app/config.json:echo eyo" \
    -splay 5s
```