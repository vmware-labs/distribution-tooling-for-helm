# Distribution Tooling for Helm

`dt`, is a set of utilities and Helm Plugin for making offline work with Helm charts easier. It is meant to be used for creating reproducible and relocatable packages for Helm charts that can be easily be moved around registries without hassles. This is particularly useful for distributing Helm charts into airgapped environments like those used by Federal governments.

This tools uses [HIP-15](https://github.com/helm/community/blob/main/hips/hip-0015.md) and the, currently proposed, [images lock file HIP (PR)](https://github.com/helm/community/pull/281) as a foundation. Hence, it does require Helm charts to contain an annotation that provides the full list of container images that a Helm chart might need for its usage independently on the bootstrapping configuration. 

[Bitnami Helm charts](https://github.com/bitnami/charts) are fully annotated to support this tooling, but you can also use this same toolset with any other Helm charts that might use any other annotation to list the container images required, like for example Helm charts using [artifact.io/images annotation](https://artifacthub.io/docs/topics/annotations/helm/).

## Installation

You can build this tool with the following command. Golang 1.20 or above is needed to compie. [golangci-lint](https://golangci-lint.run/usage/install/) is used for linting.

```sh
make build
```

You can also verify the build by running the unit tests:

```sh
make test
```

## Usage

The following sections list the most common commands and its usage. This tool can be used either standalone or through the Helm plugin.

For the sake of following this guide, lets pull one of the Bitnami Helm charts into an examples folder:

```
bash -c "mkdir examples & acme.com/federalo/bitnamicharts/mongodb -d examples --untar" 
```

### Wrapping and unwrapping Helm charts

The two simplest and most powerful commands on this tool are `wrap` and `unwrap`. With these two commands you can relocate any Helm chart with just two lines. 

**Wrapping a chart** consists on packaging the chart into the usual `tar.gz`, downloading all the chart dependencies from the source registry and wrapping everything together into a single file. That file can be distributed around in whatever way you want (e.g. USB stick) to then later be unwrapped into a destination registry. This process is commonly referred to as _relocating a Helm chart_.

Following with the example above:
```sh
bin/dt wrap examples/mariadb
INFO[0000] Wrapping chart "examples/mariadb"
INFO[0000] Found Images.lock file at "/Users/martinpe/workspace/imagelock/examples/mariadb/Images.lock"
INFO[0004] chart "examples/mariadb" lock is valid
 SUCCESS  Image docker.io/bitnami/mariadb:10.11.4-debian-11-r12 (linux/amd64) saved to "/Users/martinpe/workspace/imagelock/examples/mariadb/images/89c8d981772390150b21bc5d19d2da78535d16a3b7c738e874794d646096c513.tar"
 SUCCESS  Image docker.io/bitnami/mariadb:10.11.4-debian-11-r12 (linux/arm64) saved to "/Users/martinpe/workspace/imagelock/examples/mariadb/images/a8476d4ce4deaf68239b801f4ed5db28618687cbd573a9ad3ddc820cb1449861.tar"
 SUCCESS  Image docker.io/bitnami/mysqld-exporter:0.14.0-debian-11-r138 (linux/amd64) saved to "/Users/martinpe/workspace/imagelock/examples/mariadb/images/dc3b84db6824339fd18d69c442b997c13b5cffa9bedaf226cadaa8e5d1bfeede.tar"
 SUCCESS  Image docker.io/bitnami/mysqld-exporter:0.14.0-debian-11-r138 (linux/arm64) saved to "/Users/martinpe/workspace/imagelock/examples/mariadb/images/600b580165eb9cacc7f5c149de25672f00be68c05806bfe1cb87d7c514919b2a.tar"
 SUCCESS  Image docker.io/bitnami/os-shell:11-debian-11-r2 (linux/amd64) saved to "/Users/martinpe/workspace/imagelock/examples/mariadb/images/3170f51544e17d1eceae3c6c33e50f93b6e683ddb91e8bbd009ba2cf1803e78b.tar"
 SUCCESS  Image docker.io/bitnami/os-shell:11-debian-11-r2 (linux/arm64) saved to "/Users/martinpe/workspace/imagelock/examples/mariadb/images/2042810cb1f5e2948d8052abdb70bd12fe677fa98245f455b049d80f61414388.tar"
INFO[0023] All images pulled successfully
INFO[0023] Compressing "/Users/martinpe/workspace/imagelock/examples/mariadb" into "/Users/martinpe/workspace/imagelock/examples/mariadb-12.2.8.tar.gz"
INFO[0032] Succeeded
INFO[0032] Chart wrapped into "/Users/martinpe/workspace/imagelock/examples/mariadb-12.2.8.tar.gz"
```

A wrap file of ~360Mb containing the Helm chart and all its dependencies is generated and ready to be distributed:

```sh
ls -l /Users/martinpe/workspace/imagelock/examples/mariadb-12.2.8.tar.gz
-rw-r--r--  1 martinpe  staff  362370049 Jul 31 11:27 /Users/martinpe/workspace/imagelock/examples/mariadb-12.2.8.tar.gz
370049
```

**Unwrapping a Helm chart** can be done either locally or to a target registry, being the latter the most powerful option. By unwrapping the Helm chart to a target registry the `dt` tool will unwrap the wrapped file, proceed to push the container images into the target registry that you have specified, relocate the references from the Helm chart to the provided registry and finally push the relocated Helm chart to the registry as well. 

At that moment your Helm chart will be ready to be used from the target registry without any dependencies to the source. 

```sh
bash-3.2$ bin/dt unwrap examples/mariadb-12.2.8.tar.gz ghcr.io/mpermar
INFO[0000] Inflating "examples/mariadb-12.2.8.tar.gz"
INFO[0001] Chart decompressed to temporary location "/var/folders/mn/j41xvgsx7l90_hn0hlwj9p180000gp/T/at-wrap2644929001"
INFO[0001] Relocating "/var/folders/mn/j41xvgsx7l90_hn0hlwj9p180000gp/T/at-wrap2644929001" with prefix "ghcr.io/mpermar"
INFO[0001] Helm chart relocated successfully
Do you want to push the wrapped images into the OCI registry? [y/N]: Yes
...
INFO[0047] All images pushed successfully
INFO[0051] chart "/var/folders/mn/j41xvgsx7l90_hn0hlwj9p180000gp/T/at-wrap2644929001" lock is valid
INFO[0051] Chart unwrapped successfully
```

### Create an images lock

An images lock file, a.k.a. `Images.lock` is a new file that gets created inside the directory as per [this HIP submission](https://github.com/helm/community/pull/281) to Helm community. The `Images.lock` file contains the list of all the container images annotated within a Helm chart's `Chart.yaml` manifest, inclusing also all the images from its subchart dependencies. Along with the images, some other metadata useful for automating processing and relocation is also included

So, for example starting with the MongoDB Helm chart that we downloaded earlier, which has `images` annotation like this:

```
annotations:
    category: Database
    images: |
        - image: docker.io/bitnami/mongodb-exporter:0.39.0-debian-11-r10
          name: mongodb-exporter
        - image: docker.io/bitnami/nginx:1.25.1-debian-11-r5
          name: nginx
        - image: docker.io/bitnami/bitnami-shell:11-debian-11-r131
          name: bitnami-shell
        - image: docker.io/bitnami/mongodb:6.0.7-debian-11-r0
          name: mongodb
        - image: docker.io/bitnami/kubectl:1.25.11-debian-11-r5
          name: kubectl
    licenses: Apache-2.0
```
We can run the following command to create the `Images.lock` 

```
bin/dt images lock examples/mongodb
```

And it should look similar to this

```
cat examples/mongodb/Images.lock

apiversion: v0
kind: ImagesLock
metadata:
    generatedAt: "2023-07-14T08:37:06.571243Z"
    generatedBy: Distribution Tooling for Helm
chart:
    name: wordpress
    version: 16.1.26
images:
    - name: apache-exporter
      image: docker.io/bitnami/apache-exporter:0.13.4-debian-11-r14
      digests:
        - digest: sha256:a0440cef5a2116537147cae37e9797b335fbd0c98f4f31994d3b59b99f430627
          arch: linux/amd64
        - digest: sha256:465fbf230325b671469a95c057d3f1f4ed0a412e55e38680bfae1be08bd4d8ce
          arch: linux/arm64
      chart: wordpress
    - name: wordpress
      image: docker.io/bitnami/wordpress:6.2.2-debian-11-r29
      digests:
        - digest: sha256:50ae02430d06c4ad214b0f7fa57d3a4933559aa9175e028da9bf4315b89cd52b
          arch: linux/amd64
        - digest: sha256:b1cae81ddda673d93f8d197ece9e23f0ac9d5d3611f576d399adf3e8ee0f3ad5
          arch: linux/arm64
      chart: wordpress
    - name: os-shell
      image: docker.io/bitnami/os-shell:11-debian-11-r2
```

By default `Images.lock` creation expects an `images` annotation in your Helm chart. However this can be overriden by the `annotations-key` flag. This is useful for example for using Helm charts that have different annotations like `artifacthub.io/images`. You can use this flag with most of the commands in this guide.

```
bin/dt images lock ../charts/jenkins --annotations-key artifacthub.io/images
```

### Verify an images lock

Verifies an `Images.lock` file against a given Helm chart. This will check that upstream container images defined on the Helm chart match the information of the actual lock file.

```
bin/dt images verify examples/mongodb
```

### Pull images

Based on the `Images.lock` file, this command downloads all listed images into the `images/` subfolder.

```
bin/dt images pull examples/mongodb
```

Then, in the `images` folder we should have something like

```
ls -1 examples/mongodb/images

01144ed93f58f2eaa4794dd7f1c518ecee6aba15c7df91afd9fd1795bc8430bd.tar
023be9da1285a22b4cb7a858d06c86eaf129318bd12341ed48c3a07843e9e68c.tar
286510aac46b99558b9821449470a352532938924ff987f936555dab02351a6c.tar
34a829cfba10b360ffd133d59677954bc492d19f1eb1e0d47ff2973dfc2f83bf.tar
4359dc7199dcb84391aae8ae8f2549e38ee6e4a0ebc7d50394a5f6a2529b80ba.tar
569c93a55e00f35ed0ca1e23618dbcc90b184bbfd2c24b4974de96d93c8c58f8.tar
7901e52ce389c25b31b450a6edcb50408a5be86610aa0822fe723cfe355de8ff.tar
c54eff3d4c0641b7ee415110150df8b05e5650416ad7898b6c5bfbcc6686c72c.tar
ea1002c5fb2999ec6dfdbf286dd5ec07e784119c9473d450120958c85ab402b9.tar
```

### Relocate a chart

This command will relocate a Helm Chaacme.com/federalcare rewriting the `Images.lock` of the Helm chart and all of its subchart dependencies. Additionally, it will change the `Chart.yaml` annotations, and any images used inside the `values.yaml` file. All these on subchart dependencies as well.

For example 

```
bin/dt chart relocate examples/mongodb acme.com/federal
```

And can check that references have changed

```
cat examples/mongodb/Images.lock |grep image

images:
      image: acme.com/federal/bitnami/mongodb:6.0.7-debian-11-r0
      image: acme.com/federal/bitnami/kubectl:1.25.11-debian-11-r5
      image: acme.com/federal/bitnami/nginx:1.25.1-debian-11-r5
      image: acme.com/federal/bitnami/mongodb-exporter:0.39.0-debian-11-r10
      image: acme.com/federal/bitnami/bitnami-shell:11-debian-11-r131
```

### Push images

Based on the `Images.lock` file, this command pushes all images (that must have been previously pulled into the `images` subfolacme.com/federalelm Chart is pointing to.

This command is typically executed after having relocated a chart.

```
bin/dt images push examples/mongodb
```


### Annotate a chart (DEV)

`Images.lock` creation relies on the existence of the special images annotation inside `Chart.yaml` If you have a Helm chart that does not contain any annotations, this command can be used to generate an annotation with tentative list of images. It's important to note that this list is a **best-effort** as the list of images is guessed from `values.yaml` and this can be incomplete and error-prone as the configuration in `values.yaml` is very variable.


```
bin/dt chart annotate examples/mongodb
```

# Full Airgap Relocation Example

Let's say we have the [Bitnami MariaDB Helm chart](https://github.com/bitnami/charts/tree/main/bitnami/mariadb) and we need to relocate it to `acme.com/federal`. This is a full example using the Distribution Tooling for Helm.

```
# Get the chart with its dependencies
$> acme.com/federalo/bitnamicharts/mariadb -d examples --untar

$> helm dt images lock examples/mariadb
INFO[0000] Generating images lock for chart "examples/mariadb"
INFO[0003] Images.lock file written

# We can verify all is good
$> helm dt images verify examples/mariadb
INFO[0003] chart "examples/mariadb" lock is valid.

# Pull the image tarballs
$> helm dt images pull examples/mariadb
INFO[0001] Saving image mariadb.mariadb (docker.io/bitnami/mariadb:10.11.4-debian-11-r12) linux/amd64 to "/Users/martinpe/workspace/distribution-tooling-for-helm/examples/mariadb/images/89c8d981772390150b21bc5d19d2da78535d16a3b7c738e874794d646096c513.tar"
INFO[0005] Saving image mariadb.mariadb (docker.io/bitnami/mariadb:10.11.4-debian-11-r12) linux/arm64 to "/Users/martinpe/workspace/distribution-tooling-for-helm/examples/mariadb/images/a8476d4ce4deaf68239b801f4ed5db28618687cbd573a9ad3ddc820cb1449861.tar"
INFO[0009] Saving image mariadb.mysqld-exporter (docker.io/bitnami/mysqld-exporter:0.14.0-debian-11-r138) linux/amd64 to "/Users/martinpe/workspace/distribution-tooling-for-helm/examples/mariadb/images/dc3b84db6824339fd18d69c442b997c13b5cffa9bedaf226cadaa8e5d1bfeede.tar"
INFO[0011] Saving image mariadb.mysqld-exporter (docker.io/bitnami/mysqld-exporter:0.14.0-debian-11-r138) linux/arm64 to "/Users/martinpe/workspace/distribution-tooling-for-helm/examples/mariadb/images/600b580165eb9cacc7f5c149de25672f00be68c05806bfe1cb87d7c514919b2a.tar"
INFO[0014] Saving image mariadb.os-shell (docker.io/bitnami/os-shell:11-debian-11-r2) linux/amd64 to "/Users/martinpe/workspace/distribution-tooling-for-helm/examples/mariadb/images/3170f51544e17d1eceae3c6c33e50f93b6e683ddb91e8bbd009ba2cf1803e78b.tar"
INFO[0016] Saving image mariadb.os-shell (docker.io/bitnami/os-shell:11-debian-11-r2) linux/arm64 to "/Users/martinpe/workspace/distribution-tooling-for-helm/examples/mariadb/images/2042810cb1f5e2948d8052abdb70bd12fe677fa98245f455b049d80f61414388.tar"
INFO[0017] All images pulled successfully
...

# Relocate the images
$> helm dt chart relocate examples/mariadb acme.com/federal

# Push the images to the new registry
$> helm dt images push examples/mariadb
INFO[0000] Pushing image "acme.com/federal/bitnami/wordpress:6.2.2-debian-11-r26"
INFO[0002] Pushing image "acme.com/federal/bitnami/apache-exporter:0.13.4-debian-11-r12"
INFO[0003] Pushing image "acme.com/federal/bitnami/bitnami-shell:11-debian-11-r132"
INFO[0004] Pushing image "acme.com/federal/bitnami/memcached-exporter:0.13.0-debian-11-r8"
INFO[0006] Pushing image "acme.com/federal/bitnami/bitnami-shell:11-debian-11-r130"
INFO[0007] Pushing image "acme.com/federal/bitnami/memcached:1.6.21-debian-11-r4"
INFO[0008] Pushing image "acme.com/federal/bitnami/mariadb:10.11.4-debian-11-r0"
INFO[0010] Pushing image "acme.com/federal/bitnami/mysqld-exporter:0.14.0-debian-11-r125"
INFO[0011] Pushing image "acme.com/federal/bitnami/bitnami-shell:11-debian-11-r123"

# Push the chart to the new registry
# This needs some cleanup and should be improved in future commands
$> rm -rf examples/mariadb/images
$> helm pack examples/mariadb
Successfully packaged chart and saved it to: mariadb-12.2.8.tgz
$> helm push mariadb-12.2.8.tgz oci://acme.com/federal
Pushed: acme.com/federal/mariadb:12.2.8
Digest: sha256:0f9bf306955ffdb49e0f2cfb90cbe652e4632578f4290815a396286475bb8c42

# Pull the relocated Helm chart
$> helm pull acme.com/federal/mariadb -d examples/relocated --untar
Pulled: acme.com/federal/mariadb:12.2.8
Digest: sha256:0f9bf306955ffdb49e0f2cfb90cbe652e4632578f4290815a396286475bb8c42

# Verify the relocated Helm chart
$> helm dt images verify examples/relocated/mariadb
INFO[0003] chart "examples/relocated/mariadb" lock is valid.
```
