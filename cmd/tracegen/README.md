# tracegen

`tracegen` is a utility that can generate a steady flow of simple traces useful for performance tuning.
Traces are produced concurrently from one or more worker goroutines. Run with `-h` to see all cli flags.

The binary is available from the Releases page, as well as a Docker image:

```sh
$ docker run jaegertracing/jaeger-tracegen -service abcd -traces 10
```

Notice, however, that by default the generator uses the UDP exporter of `jaeger-client-go`,
which sends data to `localhost`, i.e. inside the networking namespace of the container itself,
which obviously doesn't go anywhere. You can use the environment variables supported by
[jaeger-client-go][env] to instruct the SDK where to send the data, for example to switch
to HTTP by setting `JAEGER_ENDPOINT`.

See example in the included [docker-compose](./docker-compose.yml) file.

[env]: https://github.com/jaegertracing/jaeger-client-go#environment-variables
