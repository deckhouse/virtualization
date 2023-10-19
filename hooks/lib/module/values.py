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

def get_value(path: str, values: dict, default=None):
    def get(keys: list, values: dict, default):
        if len(keys) == 1:
            if not isinstance(values, dict):
                return default
            return values.get(keys[0], default)
        if not isinstance(values, dict) or values.get(keys[0]) is None:
            return default
        if values.get(keys[0]) is None:
            return default
        return get(keys[1:], values[keys[0]], default)
    keys = path.lstrip(".").split(".")
    return get(keys, values, default)

def set_value(path: str, values: dict, value) -> None:
    """
    Functions for save value to dict.

    Example:
        path = "virtualization.internal.dvcr.cert"
        values = {"virtualization": {"internal": {}}}
        value = "{"ca": "ca", "crt"="tlscrt", "key"="tlskey"}"

        result values = {"virtualization": {"internal": {"dvcr": {"cert": {"ca": "ca", "crt":"tlscrt", "key":"tlskey"}}}}}
    """
    def set(keys: list, values: dict, value):
        if len(keys) == 1:
            values[keys[0]] = value
            return
        if values.get(keys[0]) is None:
            values[keys[0]] = {}
        set(keys[1:], values[keys[0]], value)
    keys = path.lstrip(".").split(".")
    return set(keys, values, value)

def delete_value(path: str, values: dict) -> None:
    if get_value(path, values) is None:
        return
    keys = path.lstrip(".").split(".")
    def delete(keys: list, values: dict) -> None:
        if len(keys) == 1:
            values.pop(keys[0])
            return
        delete(keys[1:], values[keys[0]])
    return delete(keys, values)