#
# Copyright 2023 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import base64
import os
import json


def base64_encode(b: bytes) -> str:
    return str(base64.b64encode(b), encoding='utf-8')


def base64_decode(s: str) -> str:
    return str(base64.b64decode(s), encoding="utf-8")


def base64_encode_from_str(s: str) -> str:
    return base64_encode(bytes(s, 'utf-8'))


def json_load(path: str):
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
    return data


def get_dir_path() -> str:
    return os.path.dirname(os.path.abspath(__file__))


def is_base64(s):
    try:
        base64_decode(s)
        return True
    except base64.binascii.Error:
        return False
