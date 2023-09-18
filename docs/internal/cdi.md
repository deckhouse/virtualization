# CDI

### Генерация crd
```bash
mkdir tmp
export VERSION="1.57.0"
git clone --depth 1 --branch v${VERSION} https://github.com/kubevirt/containerized-data-importer.git tmp/cdi
cd tmp/cdi
git apply ../../images/cdi-artifact/patches/*.patch
make manifests
yq e '. | select(.kind == "CustomResourceDefinition")' _out/manifests/release/cdi-operator.yaml > ../../crds/cdi.yaml
cd ../../
rm -rf tmp/cdi
```
