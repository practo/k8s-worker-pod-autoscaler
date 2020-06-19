## Golang Test Image

Test image used for `make test`.

Current image being used: `practodev/golang:1.14.2-alpine-test`

### Why?
It keeps all the test dependencies installed on top of golang package

### Built using
```
cd 1.14.2-alpine-test
docker build -t practodev/golang:1.14.2-alpine-test .
```
