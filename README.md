OpenShift OLM Catalog Validator
==

## Overview

It is an external validator which can be used to ensure that an OLM bundle is respecting
the specific criteria to publish in OCP. 

Note that a distribution must attend the common requirements defied to integrate the Operator project
with OLM and then, this validator will check the bundle which is intended to be published on OpenShift catalogs
built with OLM. 

The common criteria to publish are defined and implemented in the [https://github.com/operator-framework/api](https://github.com/operator-framework/api) 
which is also used by this project in order to try to ensure and respect the defined standards. Users are able to test their
bundles with the common and criteria to distributed in OLM by running `operator-sdk bundle validate ./bundle --select-optional suite=operatorframework` 
and using [Operator-SDK][operator-sdk].

**NOTE** We have an [EP in WIP](https://github.com/operator-framework/enhancements/pull/98). The idea is in the future
[Operator-SDK][operator-sdk] also be able to run this validator. 

## Install

Download the binary from the release page.

### From source-code

Run `make install` to be able to build and install the binary locally.
Note that you must have Go 1.16 version installed. 

## Usage

You can test this validator by running:

```sh
$ ocp-olm-catalog-validator <bundle-path> --optional-values="range==v4.8" --output=json-alpha1
```

Following an example of an Operator bundle which uses the removed APIs in 1.22 and is not configured accordingly:

```sh
$ ocp-olm-catalog-validator bundle/ --optional-values="file=bundle/metadata/annotations.yaml"
WARN[0000] Warning: Value memcached-operator.v0.0.1: this bundle is using APIs which were deprecated and removed in v1.22. More info: https://kubernetes.io/docs/reference/using-api/deprecation-guide/#v1-22. Migrate the API(s) for CRD: (["memcacheds.cache.example.com"]) 
ERRO[0000] Error: Value : (memcached-operator.v0.0.1) olm.maxOpenShiftVersion csv.Annotations not specified with an OCP version lower than 4.9. This annotation is required to prevent the user from upgrading their OCP cluster before they have installed a version of their operator which is compatible with 4.9. For further information see https://docs.openshift.com/container-platform/4.8/operators/operator_sdk/osdk-working-bundle-images.html#osdk-control-compat_osdk-working-bundle-images 
ERRO[0000] Error: Value : (memcached-operator.v0.0.1) this bundle is using APIs which were deprecated and removed in v1.22. More info: https://kubernetes.io/docs/reference/using-api/deprecation-guide/#v1-22. Migrate the APIs for this bundle is using APIs which were deprecated and removed in v1.22. More info: https://kubernetes.io/docs/reference/using-api/deprecation-guide/#v1-22. Migrate the API(s) for CRD: (["memcacheds.cache.example.com"]) or provide compatible version(s) via the labels. (e.g. LABEL com.redhat.openshift.versions='4.6-4.8') 
```

## How to check what is validated with this project?

The documentation ought to get done in this project source code in order to generate the Golang docs. 

## Release

Create a new tag and publish in the repository. It will call the GitHub action release and the
artifacts will be built and publish in the release page automatically after few minutes. 

[operator-sdk]: https://github.com/operator-framework/operator-sdk
