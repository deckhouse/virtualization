#
# Copyright 2024 Flant JSC
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

import bcrypt


class Htpasswd:
    def __init__(self,
                 username: str,
                 password: str,) -> None:
        self.username = username
        self.password = password

    def generate(self) -> str:
        bcrypted = bcrypt.hashpw(self.password.encode(
            "utf-8"), bcrypt.gensalt(prefix=b"2a")).decode("utf-8")
        return f"{self.username}:{bcrypted}"

    def validate(self, htpasswd: str) -> bool:
        user, hashed = htpasswd.strip().split(':')
        if user != self.username:
            return False
        return bcrypt.checkpw(self.password.encode("utf-8"), hashed.encode("utf-8"))
