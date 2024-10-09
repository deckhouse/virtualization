Specifics of kubevirt build and image naming.

Kubevirt is built as a single bundle as a virt-artifact. Then all necessary virt-* and libguestfs images are created from this artifact. It should be noted that the naming of these images is directly related to the naming of these images in the source artifact. Therefore, if you need to rename them, you should take this into consideration and make the appropriate edits in the kubevirt build code.

https://github.com/kubevirt/kubevirt/blob/v1.3.1/BUILD.bazel#L215-L224

The same thing for cdi (cdi-artifact).
