### Howto create patches for kubectl and cdi

1. Checkout kubevirt (or cdi) along with virtualization-controller. Use `$version` from werf.inc.yaml:
```shell
$ pwd
/home/user/virtualization-controller
$ cd ..
$ git checkout github.com/kubevirt/kubevirt kubevirt
$ git clone --branch v1.0.0 https://github.com/kubevirt/kubevirt.git kubevirt
```

2. Make changes in kubevirt directory.

3. Create patch:
```shell
$ cd kubevirt
$ git diff > ../virtualization-controller/images/virt-artifact/patches/my-precious-override.patch
```

4. Add a new file before pushing:
```shell
$ cd ../virtualization-controller
$ git add images/virt-artifact/patches/my-precious-override.patch
```
