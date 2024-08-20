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

PUBLIC_DOMAIN_PREFIX = "%PUBLIC_DOMAIN%://"
CLUSTER_DOMAIN_PREFIX = "%CLUSTER_DOMAIN%://"

def cluster_domain_san(san: str) -> str:
    """
    Create template to enrich specified san with a cluster domain
    :param san: San.
    :type sans: :py:class:`str`
    """
    return CLUSTER_DOMAIN_PREFIX + san.rstrip('.')


def public_domain_san(san: str) -> str:
    """
    Create template to enrich specified san with a public domain
    :param san: San.
    :type sans: :py:class:`str`
    """
    return PUBLIC_DOMAIN_PREFIX + san.rstrip('.')


def get_public_domain_san(san: str, public_domain: str) -> str:
    return f"{san.lstrip(PUBLIC_DOMAIN_PREFIX)}.{public_domain}"


def get_cluster_domain_san(san: str, cluster_domain: str) -> str:
    return f"{san.lstrip(CLUSTER_DOMAIN_PREFIX)}.{cluster_domain}"
