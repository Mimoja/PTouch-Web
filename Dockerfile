FROM golang:1.22-alpine
MAINTAINER Mimoja <git@mimoja.de>, Tyalie <git@flowerpot.me>

RUN  apk add --no-cache build-base pkgconfig libusb libusb-dev

RUN mkdir /app
WORKDIR /app
# layer for dependencies 
COPY go.mod go.sum /app/
RUN go mod download

# layer for application code
COPY . /app/

RUN go build -v .

CMD ["/app/ptouch-web"]
