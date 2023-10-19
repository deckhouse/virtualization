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

import random 
import string

def generate_random_string(length: int, letters: str) -> str:
    return ''.join(random.choice(letters) for i in range(length))

SYMBOLS = "[]{}<>()=-_!@#$%^&*.,"

def num(length: int) -> str:
    """
    Generates a random string of the given length out of numeric characters.
    :param length: length of generate string.
    :type length: :py:class:`int`
    :rtype: :py:class:`str`
    """
    return generate_random_string(length, string.digits)

def alpha(length: int) -> str:
    """
    Generates a random string of the given length out of alphabetic characters.
    :param length: length of generate string.
    :type length: :py:class:`int`
    :rtype: :py:class:`str`
    """
    return generate_random_string(length, string.ascii_letters)

def symbols(length: int) -> str:
    """
    Generates a random string of the given length out of symbols.
    :param length: length of generate string.
    :type length: :py:class:`int`
    :rtype: :py:class:`str`
    """
    return generate_random_string(length, SYMBOLS)


def alpha_num(length: int) -> str:
    """
    Generates a random string of the given length out of alphanumeric characters.
    :param length: length of generate string.
    :type length: :py:class:`int`
    :rtype: :py:class:`str`
    """
    return generate_random_string(length, string.ascii_letters + string.digits)

def alpha_num_lower_case(length: int) -> str:
    """
    Generates a random string of the given length out of alphanumeric characters without UpperCase letters.
    :param length: length of generate string.
    :type length: :py:class:`int`
    :rtype: :py:class:`str`
    """
    return generate_random_string(length, string.ascii_lowercase + string.digits)

def alpha_num_symbols(length: int) -> str:
    """
    Generates a random string of the given length out of alphanumeric characters and symbols.
    :param length: length of generate string.
    :type length: :py:class:`int`
    :rtype: :py:class:`str`
    """
    return generate_random_string(length, string.ascii_letters + string.digits + SYMBOLS)
