# About

Polarstomps is demo web application written in the [Templ](https://templ.guide/) Go library. It is meant to be used as a demo application for using ArgoCD on top of an EKS cluster. It is not a real project.

# Dependencies

* Redis
* GCS (or local emulation)

## Redis

This web application will write to Redis as an example of communicating with a datastore in GCP. This means that a locally hosted Redis is required.

## GCS

This web application will read a list of objects from a GCS bucket. This can be done locally using a GCS emulation server. Run `make gcs-auth` and look at `.envrc`.

# Running

``` sh
templ generate
go build
./polarstomps
```

---
