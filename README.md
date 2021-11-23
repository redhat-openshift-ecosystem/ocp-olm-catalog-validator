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

## Usage

You can test this validator by running:

```sh
$ ocp-olm-catalog-validator <bundle-path> --optional-values="range==v4.8" --output=json-alpha1
```

## How to check what is validated with this project?

The documentation ought to get done in this project source code in order to generate the Golang docs. 

## Release

Create a new tag and publish in the repository. It will call the GitHub action release and the
artifacts will be built and publish in the release page automatically after few minutes. 

[operator-sdk]: https://github.com/operator-framework/operator-sdk