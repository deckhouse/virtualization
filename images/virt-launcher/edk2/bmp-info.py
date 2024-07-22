#!/usr/bin/env python3

# Copyright 2024 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# An excerpt from edk2-20231115/BaseTools/Source/Python/AutoGen/GenC.py
# Usage:
# ./bmp-info.py Logo.bmp
# Width: 400
# Height: 250
# BitCount: 24
# Compression: 0

import collections
import struct
import sys

def BmpImageDecoder(Buffer):
    ImageType, = struct.unpack('2s', Buffer[0:2])
    if ImageType!= b'BM': # BMP file type is 'BM'
        print("error: file type mismatch: not a BMP file.")
        return
    BMP_IMAGE_HEADER = collections.namedtuple('BMP_IMAGE_HEADER', ['bfSize', 'bfReserved1', 'bfReserved2', 'bfOffBits', 'biSize', 'biWidth', 'biHeight', 'biPlanes', 'biBitCount', 'biCompression', 'biSizeImage', 'biXPelsPerMeter', 'biYPelsPerMeter', 'biClrUsed', 'biClrImportant'])
    BMP_IMAGE_HEADER_STRUCT = struct.Struct('IHHIIIIHHIIIIII')
    BmpHeader = BMP_IMAGE_HEADER._make(BMP_IMAGE_HEADER_STRUCT.unpack_from(Buffer[2:]))

    print("Width: %d" % BmpHeader.biWidth)
    print("Height: %d" % BmpHeader.biHeight)
    print("BitCount: %d" % BmpHeader.biBitCount)
    print("Compression: %d" % BmpHeader.biCompression)
    return

FileName = sys.argv[1]
TmpFile = open(FileName, 'rb')
Buffer = TmpFile.read()
TmpFile.close()

BmpImageDecoder(Buffer)
