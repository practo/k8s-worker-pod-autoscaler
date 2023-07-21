## Golang Test Image

Test image used for `make test`.

Current image being used: `public.ecr.aws/practo/golang:1.17.1-alpine-test`

### Why?
It keeps all the test dependencies installed on top of golang package

### Built using
```
cd 1.17.1-alpine-test
docker build -t public.ecr.aws/practo/golang:1.17.1-alpine-test .
```
