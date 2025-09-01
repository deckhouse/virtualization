# Linux-PAM Package

This package builds the Linux-PAM (Pluggable Authentication Modules) library from source. Linux-PAM provides a flexible mechanism for authenticating users.

## Source Repository

- **Repository**: https://github.com/linux-pam/linux-pam
- **Version**: v1.6.0

## Build Process

The package is built using werf v2 with the following stages:

1. **src-artifact**: Clones the source code from GitHub
2. **builder**: Builds the package using autotools on the builder/alt base image
3. **final**: Creates the final package image

## Dependencies

- gcc, git, make
- autoconf, automake, libtool
- glibc development libraries
- zlib development libraries

## Build Configuration

The package is configured with:
- Shared and static library support
- Installation to `/usr` with libraries in `lib64`
- Documentation disabled for smaller size

## Output

The built package includes:
- libpam shared libraries
- libpamc shared libraries
- libpam_misc shared libraries
- Header files and development artifacts

## Usage

Linux-PAM is used by various system services for authentication and authorization purposes.
