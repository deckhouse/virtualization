#!/usr/bin/env bash

if $(docker buildx ls | grep -q multibuilder); then
  echo "multibuilder exists"
else
  echo "Creating multibuilder"
  docker buildx create --name multibuilder --bootstrap --use
fi

# docker buildx rm multibuilder
export DOCKER_DEFAULT_PLATFORM=linux/amd64

docker buildx build -f "Dockerfile.libvirt.alt" -t virt:libvirt-artifact --platform="linux/amd64" --load . 
# docker build -t virt:libvirt-artifact -f "Dockerfile.libvirt.alt"
docker image ls | grep libvirt

echo "Run container"
echo "docker run --name libvirt --rm -it --platform=linux/amd64 virt:libvirt-artifact bash"