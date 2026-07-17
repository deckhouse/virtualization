- [Модель угроз безопасности модуля `virtualization`](#модель-угроз-безопасности-модуля-virtualization)
  - [Исходные данные и область анализа](#исходные-данные-и-область-анализа)
  - [1. Определение целей и критичных функций модуля](#1-определение-целей-и-критичных-функций-модуля)
  - [2. Архитектурная модель модуля](#2-архитектурная-модель-модуля)
  - [3. Анализ поверхности атаки модуля](#3-анализ-поверхности-атаки-модуля)
  - [4. Идентификация угроз](#4-идентификация-угроз)
  - [5. Моделирование сценариев атак](#5-моделирование-сценариев-атак)
    - [Сценарий AS-01. Побег из гостевой ВМ на узел (VM escape) через QEMU/KVM/libvirt](#сценарий-as-01-побег-из-гостевой-вм-на-узел-vm-escape-через-qemukvmlibvirt)
    - [Сценарий AS-02. Несанкционированный доступ к чужой ВМ через console/VNC/portforward](#сценарий-as-02-несанкционированный-доступ-к-чужой-вм-через-consolevncportforward)
    - [Сценарий AS-03. Воздействие на ВМ через спецификацию CRD, cloud-init и обход admission](#сценарий-as-03-воздействие-на-вм-через-спецификацию-crd-cloud-init-и-обход-admission)
    - [Сценарий AS-04. Отказ admission-валидации (`failurePolicy: Fail`) → DoS плоскости управления](#сценарий-as-04-отказ-admission-валидации-failurepolicy-fail--dos-плоскости-управления)
    - [Сценарий AS-05. Компрометация учётных данных DVCR и TLS-секретов модуля](#сценарий-as-05-компрометация-учётных-данных-dvcr-и-tls-секретов-модуля)
    - [Сценарий AS-06. Подмена образа ВМ через импорт из недоверенного источника / DVCR](#сценарий-as-06-подмена-образа-вм-через-импорт-из-недоверенного-источника--dvcr)
    - [Сценарий AS-07. Повышение привилегий до узла через привилегированные DaemonSet (virt-handler / vm-route-forge / virtualization-dra)](#сценарий-as-07-повышение-привилегий-до-узла-через-привилегированные-daemonset-virt-handler--vm-route-forge--virtualization-dra)
    - [Сценарий AS-08. Нарушение сетевой маршрутизации/изоляции ВМ через vm-route-forge](#сценарий-as-08-нарушение-сетевой-маршрутизацииизоляции-вм-через-vm-route-forge)
    - [Сценарий AS-09. Компрометация цепочки поставки образов (KubeVirt/CDI/QEMU/libvirt/edk2/DVCR)](#сценарий-as-09-компрометация-цепочки-поставки-образов-kubevirtcdiqemulibvirtedk2dvcr)
    - [Сценарий AS-10. Раскрытие информации и подавление аудита](#сценарий-as-10-раскрытие-информации-и-подавление-аудита)
    - [Сценарий AS-11. Атака на live-migration ВМ через миграционные туннели virt-handler (порты 4135–4199)](#сценарий-as-11-атака-на-live-migration-вм-через-миграционные-туннели-virt-handler-порты-41354199)
    - [Сценарий AS-12. Эскалация через USB-passthrough DRA и загрузку kernel-модулей usbip на всех узлах](#сценарий-as-12-эскалация-через-usb-passthrough-dra-и-загрузку-kernel-модулей-usbip-на-всех-узлах)
    - [Сценарий AS-13. Инъекция через cloud-init/sysprep provisioning-секреты ВМ](#сценарий-as-13-инъекция-через-cloud-initsysprep-provisioning-секреты-вм)
    - [Сценарий AS-14. Нарушение целостности через многоверсионность `VirtualMachineClass` и отсутствие conversion-webhook](#сценарий-as-14-нарушение-целостности-через-многоверсионность-virtualmachineclass-и-отсутствие-conversion-webhook)
  - [6. Оценка актуальности и формирование модели угроз](#6-оценка-актуальности-и-формирование-модели-угроз)
    - [Итоговая модель актуальных угроз](#итоговая-модель-актуальных-угроз)
    - [Меры по нейтрализации](#меры-по-нейтрализации)

<div class="page"></div>

# Модель угроз безопасности модуля `virtualization`

## Исходные данные и область анализа

<div class="tm-table tm-table-meta tm-table-scope"></div>

| Параметр | Значение |
| --- | --- |
| **Объект моделирования** | Deckhouse-модуль `virtualization` (Deckhouse Virtualization Platform, DVP), namespace `d8-virtualization` |
| **Локальный репозиторий** | `./virtualization` (ветка/коммит фиксируются при экспорте) |
| **Методика** | «Методика моделирования угроз и поверхности атаки» (`5.7-threat-modeling.md`) |
| **Каталог угроз** | БДУ ФСТЭК России — `Угрозы.csv` (УБИ.1–УБИ.11), `Угрозы-Способы.csv` (способы реализации СП.*) |
| **Stage модуля** | General Availability (`module.yaml`) |
| **Базовые требования** | Deckhouse `>= 1.74.2`; модуль `cni-cilium >= 0.0.0`; ядро Linux `>= 5.7`; CPU x86_64 c Intel‑VT (vmx) / AMD‑V (svm) |
| **Источники анализа** | `README.md`, `docs/*` (README, CONFIGURATION, ADMIN_GUIDE, USER_GUIDE, FAQ), `module.yaml`, `Chart.yaml`, `oss.yaml`, `openapi/*`, `templates/*`, `crds/*`, `images/*` (`werf.inc.yaml`, исходный код Go/C), `api/*` |

---
<div class="tm-section-gap tm-section-gap-md"></div>

## 1. Определение целей и критичных функций модуля

<div class="tm-table tm-table-meta tm-table-goals"></div>

| Параметр | Описание |
| --- | --- |
| **Наименование модуля** | `virtualization`, Deckhouse Virtualization Platform (DVP); namespace `d8-virtualization` |
| **Назначение** | Декларативное создание, запуск и управление виртуальными машинами (ВМ) и связанными ресурсами (образы, диски, снапшоты, IP/MAC-адреса, USB-устройства) внутри кластера DKP/Kubernetes. Модуль построен на форке проектов KubeVirt и CDI, использует QEMU/KVM + libvirt для запуска ВМ. Предоставляет пользовательский API (`virtualization.deckhouse.io`) и внутренний реестр образов DVCR. |
| **Режим эксплуатации** | Сетевой, распределённый, кластерный (облачный/bare-metal). Компоненты функционируют как контроллеры/операторы/DaemonSet внутри кластера; ВМ исполняются в подах (`virt-launcher`) на узлах с поддержкой аппаратной виртуализации. |
| **Среда исполнения** | DKP/Kubernetes (`>= 1.74.2`), namespace `d8-virtualization`; узлы Linux (ядро `>= 5.7`, `/dev/kvm`); CNI `cilium`; контейнерные образы Deckhouse; Kubernetes API; хранилище PVC через CSI (sds-replicated/local, csi-nfs, csi-ceph) для DVCR. |
| **Основные функции** | Реконсиляция ресурсов `VirtualMachine`, `VirtualDisk`, `VirtualImage`, `ClusterVirtualImage`, `VirtualMachineBlockDeviceAttachment`, `VirtualMachineClass`, `VirtualMachineSnapshot`, `VirtualDiskSnapshot`, `VirtualMachineOperation`, `VirtualMachineIPAddress`/`Lease`, `VirtualMachineMACAddress`/`Lease`, `UsbDevice`/`NodeUsbDevice`; импорт/загрузка/кэширование образов ВМ в DVCR; выделение статических IP/MAC из заданных CIDR; настройка маршрутов к ВМ (`vm-route-forge`); проброс USB-устройств (DRA); интерактивный доступ к ВМ (console/VNC/portforward/SSH) через агрегированный API; live-миграция, hotplug CPU/Memory (EE); аудит действий (EE). |
| **Критичные функции** | Изоляция гостевых ВМ от хоста и друг от друга (KVM/QEMU/libvirt, SELinux, seccomp); разграничение доступа к ресурсам ВМ через Kubernetes RBAC и admission-валидацию; защита и хранение образов/дисков в DVCR; защита учётных данных DVCR (htpasswd) и TLS-секретов; корректность аутентификации/авторизации при доступе к console/VNC/portforward; целостность сетевой маршрутизации к ВМ (`vm-route-forge`, route table 1490); корректность импорта образов из внешних источников; целостность сборки образов QEMU/libvirt/edk2/KubeVirt/CDI (supply chain). |
| **Критичные последствия** | Побег из гостевой ВМ на узел (VM escape) с компрометацией ноды и кластера; несанкционированный доступ к чужим ВМ (console/VNC/диски); раскрытие/подмена образов и дисков ВМ; компрометация учётных данных DVCR и TLS-ключей; повышение привилегий из namespace-пользователя до доступа к узлу через привилегированные DaemonSet (`virt-handler`, `vm-route-forge`, `virtualization-dra`); отказ в обслуживании плоскости управления виртуализацией или ВМ; нарушение сетевой изоляции/маршрутизации ВМ; внедрение скомпрометированных зависимостей в образы QEMU/libvirt/firmware. |
| **Объекты защиты** | Образы и диски ВМ (DVCR-блобы на PVC); пользовательские/кластерные ресурсы виртуализации (CRD); конфигурация `ModuleConfig`; учётные данные DVCR (`dvcr-secrets`: passwordRW, htpasswd, salt; `dvcr-dockercfg-rw`); TLS-секреты сервисов (`virtualization-controller-tls`, `virtualization-api-tls`, `virtualization-api-proxy-tls`, `virtualization-audit-tls`, `dvcr-tls`); корневой CA модуля (`virtualization-ca`); ServiceAccount-токены и RBAC-права компонентов; admission-вебхуки и политики; route table 1490 и сетевой путь к ВМ; журналы аудита; firmware (OVMF/edk2), бинарники QEMU/libvirt и цепочка сборки. |
| **Категории субъектов** | Внешний нарушитель без доступа к кластеру (0) — например, через сетевые точки входа (ingress upload-proxy DVCR, экспонированные сервисы); пользователь Kubernetes с правами на ВМ в namespace (`user`/`editor`/`privileged-user`) (1); гостевая ОС внутри ВМ как недоверенный субъект (1); оператор/администратор DVP с правами `admin`/`cluster-editor`/`manage` и доступом к `ModuleConfig` (2); внутренние компоненты модуля и Kubernetes API (доверенные сервисные субъекты); внешние источники образов (HTTP/registry) и источники сборки (`SOURCE_REPO`, `GOPROXY`, registry) — ограниченно доверенные внешние системы. |
| **Реализованные меры защиты, выявленные по исходным данным** | Запуск большинства Go-компонентов от непривилегированного пользователя `64535` на distroless/scratch-базе; `kube-rbac-proxy` (SubjectAccessReview) для защиты метрик контроллера/DVCR/cdi-operator; агрегированный `virtualization-api` с delegating authentication/authorization и проксированием к virt-api по клиентскому сертификату; `ValidatingAdmissionPolicy` `failurePolicy: Fail`, ограничивающая доступ к внутренним CRD только системным ServiceAccount; ValidatingWebhook на пользовательские CRD и `ModuleConfig`; разделение RBAC-ролей (user/editor/privileged-user/admin/cluster-editor, rbacv2-агрегация); сборка QEMU с seccomp/SELinux и ограниченным whitelist блочных драйверов; OVMF с UEFI Secure Boot/revocation list; CA модуля и per-service TLS; SELinux-политики на узлах; фиксация версий зависимостей в `oss.yaml` и `werf-giterminism.yaml`. |

---

<div class="page"></div>

## 2. Архитектурная модель модуля

Архитектурная модель сформирована по указаниям раздела 2 методики. Модуль состоит из трёх функциональных подсистем: **API** (пользовательский API виртуализации), **CORE** (форк KubeVirt + CDI, QEMU/KVM/libvirt) и **DVCR** (реестр образов), а также вспомогательных компонентов безопасности и сети.

**Компоненты модуля и границы доверия:**

<div class="tm-table tm-table-architecture"></div>

| Компонент | Тип | Назначение | Уровень доверия | Граница доверия |
| --- | --- | --- | --- | --- |
| **Helm/werf templates модуля** | Внутренний компонент | Формирование namespace, RBAC, Service, Deployment/DaemonSet, Secret, ConfigMap, webhook, admission-policy, NodeGroupConfiguration | Доверенный субъект при контроле релизного артефакта | Да, между релизным артефактом и Kubernetes API |
| **virtualization-api** (Deployment) | Внутренний компонент | Агрегированный extension API-server `virtualization.deckhouse.io`; subresources console/vnc/portforward; проксирование к virt-api по клиентскому сертификату; delegating authn/authz | Доверенный субъект (реализует ФБ аутентификации/авторизации), обрабатывает запросы недоверенных пользователей | Да, между пользователем/kube-apiserver и virt-api |
| **virtualization-controller** (Deployment) | Внутренний компонент | Реконсиляция CRD модуля (VM, VD, VI, CVI, VMBDA, VMClass, snapshots, IP/MAC, USB); admission/conversion webhooks; запуск импорта образов в DVCR | Ограниченно доверенный субъект: обрабатывает недоверенные пользовательские спецификации CRD | Да |
| **virtualization-audit** (Deployment, EE) | Внутренний компонент | Аудит действий над ресурсами виртуализации (события Kubernetes audit) | Доверенный субъект (ФБ аудита) | Да |
| **kube-api-rewriter** (sidecar) | Внутренний компонент | Переименование API-групп KubeVirt↔Deckhouse в трафике controller/operator/webhook | Доверенный субъект | Нет для loopback `127.0.0.1`, Да для kube-apiserver |
| **kube-rbac-proxy** (sidecar) | Внутренний компонент | Ограничение доступа к метрикам (`prometheus-metrics`) через SubjectAccessReview | Доверенный субъект (ФБ авторизации) | Да |
| **DVCR** (Deployment) | Внутренний компонент | Реестр образов ВМ (форк docker/distribution v2.8.3 + dvcr-cleaner); хранение/кэширование образов; backend PVC | Ограниченно доверенный субъект: обрабатывает данные образов и аутентификацию push/pull | Да |
| **dvcr-importer** (Pod, эфемерный) | Внутренний компонент | Импорт образов из внешних источников (HTTP/registry, qcow2/raw через qemu-img/nbdkit) в DVCR | Ограниченно доверенный субъект: обрабатывает недоверенные внешние образы | Да |
| **dvcr-uploader** (Pod/upload-proxy) | Внутренний компонент | Приём загружаемых пользователем образов (upload server, TLS, client-cert) | Ограниченно доверенный субъект: принимает недоверенные пользовательские данные | Да |
| **CDI (cdi-operator/apiserver/deployment/cdi-importer/cloner/uploader)** | Внутренний компонент / форк upstream | Управление дисками и импортом данных в PVC (Containerized Data Importer v1.60.3) | Ограниченно доверенный субъект | Да |
| **KubeVirt CORE (virt-operator/virt-api/virt-controller/virt-handler/virt-exportproxy)** | Внутренний компонент / форк upstream | Управление жизненным циклом ВМ (KubeVirt v1.6.2); `virt-handler` — node-agent на каждой ноде с ВМ | Ограниченно доверенный субъект; `virt-handler` — привилегированный | Да |
| **virt-launcher** (Pod на ноду ВМ) | Внутренний компонент с повышенными правами | Контейнер исполнения ВМ: libvirt (`virtqemud`), QEMU v9.2.0, swtpm, OVMF; запускается **от root**, `cap_net_bind_service`, доступ к `/dev/kvm` | Привилегированный компонент, обрабатывает недоверенную гостевую ОС | Да, между гостевой ВМ и узлом |
| **virt-handler** (DaemonSet) | Внутренний компонент с повышенными правами | Node-agent KubeVirt: `privileged`, `hostNetwork`+`hostPID`, `runAsUser 0`, hostPath RW к `/var/lib/kubelet`, `/var/run/kubevirt*`, `/var/lib/kubevirt-node-labeller` | Привилегированный доверенный компонент | Да, между Pod и узлом |
| **vm-route-forge** (DaemonSet) | Внутренний компонент с повышенными правами | Настройка маршрутов к ВМ в таблице маршрутизации `1490` через Cilium; `hostNetwork`, `privileged`, root, eBPF; `NET_ADMIN` | Привилегированный доверенный компонент | Да, между Pod и сетевым стеком узла |
| **virtualization-dra / virtualization-dra-usb** (DaemonSet) | Внутренний компонент с повышенными правами | DRA kubelet-плагин для проброса USB (`go-usbip`, `usbipd`); `hostNetwork`, `privileged`, root; initContainer с `SYS_MODULE`; hostPath RW к `/lib/modules`, `/sys`, `/var/run`, kubelet plugins | Привилегированный доверенный компонент | Да, между Pod, kubelet и устройствами узла |
| **bounder** | Внутренний компонент | Статический probe/placeholder бинарь (C, musl static) | Доверенный объект, без сети/привилегий | Нет |
| **d8 CLI (`src/cli`)** | Клиентский инструмент | Команды console/vnc/ssh/scp/portforward/ansibleinventory/collectdebuginfo к ВМ через API | Доверенный клиент пользователя | Да, между рабочей станцией пользователя и API |
| **Kubernetes API** | Внешняя система | Хранилище CRD, Secret, ConfigMap, Lease, Node; admission, RBAC, audit | Ограниченно доверенная внешняя система | Да |
| **kubelet (узлов)** | Внешняя система | Регистрация DRA-плагина, запуск Pod ВМ, device plugins | Ограниченно доверенная внешняя система | Да |
| **CNI cilium / сетевой стек узла** | Внешняя система | Сетевая связность Pod/ВМ, eBPF, маршрутизация | Ограниченно доверенная внешняя система | Да |
| **Хранилище (CSI PVC)** | Внешняя система | Хранение блобов DVCR и дисков ВМ | Ограниченно доверенная внешняя система | Да |
| **Внешние источники образов (HTTP/registry)** | Внешняя система | Источник образов ВМ при импорте | Недоверенная внешняя система | Да |
| **Гостевая ОС внутри ВМ** | Внешний субъект | Произвольный код пользователя/нарушителя внутри ВМ | Недоверенный субъект | Да, между гостем и хостом |
| **Container Registry и source repositories (`SOURCE_REPO`, `GOPROXY`, `PACKAGE_CLONE_REPO`)** | Внешние/сборочные системы | Источник OCI-образов, исходного кода KubeVirt/CDI/QEMU/libvirt/edk2 и зависимостей сборки | Ограниченно доверенная система поставки | Да |
| **Prometheus/Grafana** | Внешняя система мониторинга | Сбор и отображение метрик компонентов | Ограниченно доверенная система | Да |
| **cert-manager / global HTTPS** | Внешняя система | Выпуск TLS-сертификатов для ingress upload-proxy | Ограниченно доверенная система | Да |






**Основные интерфейсы и потоки данных (с портами и протоколами):**

<div class="tm-table tm-table-flows"></div>

| Источник | Получатель | Протокол/формат, порт | Назначение | Доверенность данных |
| --- | --- | --- | --- | --- |
| Пользователь / `d8` CLI / kube-apiserver | virtualization-api | HTTPS (aggregation), TLS, `:8443` | Запросы к API виртуализации и subresources console/vnc/portforward | Недоверенные данные |
| virtualization-api | virt-api (KubeVirt) | HTTPS + клиентский сертификат (`proxy-client-cert`) | Проксирование console/vnc/portforward с аутентификацией | Ограниченно доверенные данные |
| kube-apiserver (admission) | virtualization-controller | HTTPS AdmissionReview v1, Service `virtualization-controller:443` (вебхук-сервер `0.0.0.0:9443`) | Валидация/дефолтинг CRD и `ModuleConfig` | Недоверенные данные |
| kube-api-rewriter (sidecar) | virtualization-controller | TCP loopback `127.0.0.1:9443` ↔ `0.0.0.0:24192` | Передача вебхук-трафика с переименованием групп | Производные от недоверенных |
| virtualization-controller | kube-apiserver (через rewriter `127.0.0.1:23915`) | Kubernetes watch/list/get/update | Реконсиляция CRD, Pod, PVC, Secret, Lease | Ограниченно доверенные данные |
| Пользователь | kube-apiserver (CRD) | Kubernetes API YAML/JSON | Создание/изменение VM/VD/VI/VMBDA/VMClass | Недоверенные данные |
| dvcr-importer | Внешний источник (HTTP/registry) | HTTP/HTTPS/registry v2 | Загрузка образов ВМ | Недоверенные данные |
| Пользователь / ingress | dvcr-uploader (upload-proxy) | HTTPS, client-cert, ingress-tls | Загрузка пользовательских образов | Недоверенные данные |
| virtualization-controller / CDI | DVCR | HTTP/HTTPS registry v2 (push/pull, basic auth htpasswd) | Хранение/получение образов | Ограниченно доверенные данные |
| DVCR | PVC | файловая ФС | Хранение блобов образов | Ограниченно доверенные данные |
| Prometheus | kube-rbac-proxy → компонент | HTTPS scrape (SubjectAccessReview), напр. `:8082` | Получение метрик | Ограниченно доверенные данные |
| virt-handler | kubelet / узел | hostPath, `/var/run/kubevirt*`, `/var/lib/kubelet`; порты `4100` (метрики/healthz), `4101` (console), `4135-4199` (миграция) | Управление ВМ на узле, live-миграция | Привилегированное действие |
| virt-launcher | KVM/libvirt/QEMU на узле | `/dev/kvm`, vhost, virtio, libvirt socket | Исполнение гостевой ВМ | Недоверенная гостевая ОС → привилегированный хост |
| vm-route-forge | Сетевой стек узла | netlink, route table `1490`, eBPF; health `127.0.0.1:4105`, pprof `:4106` (debug) | Маршрутизация трафика к ВМ | Привилегированное действие |
| virtualization-dra | kubelet | gRPC over UNIX socket (plugins_registry); health `:4107`, usbipd `:4280` (podIP) | Регистрация DRA-плагина, проброс USB | Привилегированное действие |
| virtualization-audit | kube-apiserver / audit | HTTPS `--secure-port=8443`, TLS `/etc/virtualization-audit/certificate/` | Обработка событий аудита | Ограниченно доверенные данные |
| werf build | Registry / source repos | OCI images, git clone (`SOURCE_REPO`), Go modules (`GOPROXY`) | Сборка и обновление образов и зависимостей | Ограниченно доверенные данные поставки |

<div class="page"></div>

**Используемые сторонние компоненты и зависимости (с фиксацией версий, источник — `oss.yaml`, `Chart.yaml`, `werf.inc.yaml`):**

<div class="tm-table tm-table-dependencies"></div>

| Компонент | Версия / источник | Назначение | Замечания безопасности |
| --- | --- | --- | --- |
| KubeVirt (форк `deckhouse/3p-kubevirt`) | upstream v1.6.2 → тег `v1.6.2-v12n.40`<br><br>Источники: `oss.yaml`, `build/components/versions.yml`, `images/virt-artifact`.<br><br>SBOM Go-биндингов:<br>`tmp/sbom-output/main/{virtlauncher,`<wbr>`virthandler,`<wbr>`virtcontroller,`<wbr>`virtoperator,`<wbr>`virtapi}/packages.tsv`.<br><br>Модуль `kubevirt.io/kubevirt` помечен как `root` без явной semver. Нативный слой `virt-launcher`:<br>`virtlauncher/`<wbr>`native-artifacts.tsv`, qemu-system/qemu-img + libvirt.so.0.10009.0. | Управление жизненным циклом ВМ (CORE) | Форк `github.com/deckhouse/3p-kubevirt` с локальными патчами, shallow-clone из `SOURCE_REPO`.<br><br>API-группы переименовываются через kube-api-rewriter.<br><br>Открыто: привязка нативного слоя к CVE, наполнение OpenVEX-statements (`tmp/sbom-output/main/virtlauncher/`<wbr>`openvex.json`, `statements: []`) и ревью отклонений от upstream. |
| CDI — Containerized Data Importer (форк `deckhouse/3p-containerized-data-importer`) | upstream v1.60.3 → тег `v1.60.3-v12n.19` <br>(`oss.yaml`, `build/components/versions.yml`, `images/cdi-*`); API `containerized-data-importer-api v1.63.1` | Управление дисками и импортом данных | Форк `github.com/deckhouse/3p-containerized-data-importer` с локальными патчами; `cdi-importer`/`cdi-uploader` обрабатывают недоверенные внешние образы. |
| QEMU (патченый) | v9.2.0 (`oss.yaml`, `images/qemu`) | Эмуляция/виртуализация (x86_64-softmmu) | Сборка с KVM/vhost/seccomp/SELinux, ограниченным whitelist блочных драйверов; патчи применяются локально. |
| Libvirt | v10.9.0 (`oss.yaml`, `images/virt-launcher`) | Управление виртуализацией (`virtqemud`) | Работает в `virt-launcher` от root. |
| edk2 / OVMF (firmware) | upstream `tianocore/edk2` + `edk2-platforms`, commit/tag-pinned (`images/edk2`) | UEFI firmware ВМ | Secure Boot, кастомный logo, UEFI revocation list; собирается из `SOURCE_REPO`. |
| docker/distribution (DVCR) | v2.8.3 (`images/dvcr`) | Реестр образов ВМ | Клонируется из `SOURCE_REPO`; basic-auth htpasswd; backend PVC. |
| deckhouse_lib_helm | 1.55.1 (`Chart.yaml`, `requirements.lock`) | Helm-библиотека шаблонов | digest зафиксирован в `requirements.lock`. |
| kube-rbac-proxy | образ Deckhouse (`templates/kube-rbac-proxy`) | Авторизация доступа к метрикам | Запуск от nobody (65534), drop ALL caps. |
| nbdkit, libnbd, qemu-img/qemu-nbd, libxml2, `file` | нативные зависимости `dvcr-artifact` (CGO_ENABLED=1) | Конвертация/импорт образов (qcow2/raw) | Большая нативная цепочка зависимостей; Trivy-слой (`tmp/sbom-output/main/cdiimporter/packages.tsv`) видит только Go-обвязку `libguestfs.org/libnbd v1.11.5` (`OS: None`), но нативный слой теперь фиксирует сами C-бинарники по SHA-256 (`cdiimporter/native-artifacts.tsv`: `usr/sbin/nbdkit`, `usr/bin/qemu-img`, `libnbd.so.0.0.0`, `libxml2.so.16.0.5`, nbdkit-плагины/фильтры) — версии из `build/components/versions.yml`. |
| base-alt-p11-binaries / packages / distroless | внутренние сборочные образы | Базовые образы и пакеты | Версии зависят от werf-сборки; в SBOM 21 итогового образа фиксируются werf-лейблы (`werf-stage-content-digest`, `werf.io/parent-stage-id`), но базовый distroless-слой не имеет ОС-инвентаризации (`Metadata.OS: None`). |

---

<div class="page"></div>

## 3. Анализ поверхности атаки модуля

Поверхность атаки сформирована на основе архитектурной модели (раздел 2). В соответствии с методикой не учитываются угрозы от аппаратных уязвимостей и root-пользователя в операционной системе узла. Доверенными считаются только данные, созданные самим модулем, и hardcoded-данные. Для интерфейсов указаны порты/протоколы, версии компонентов и признак реализации функции безопасности (ФБ).

<div class="tm-table tm-table-attack-surface"></div>

| Элемент | Компонент | Версия | ФБ / функция | Тип интерфейса | Уровень доступа | Взаимодействие | Недовер. данные |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Агрегированный API `virtualization.deckhouse.io` (`:8443`/aggregation)** | virtualization-api | KubeVirt v1.6.2 API-слой | Да — delegating authentication/authorization, TLS | Программный API (HTTPS) | Ограниченный по RBAC | Внешний/внутренний через kube-apiserver | Запросы пользователей, параметры subresources |
| **Subresources `virtualmachines/console`, `/vnc`, `/portforward`** | virtualization-api → virt-api | v1.6.2 | Да — RBAC (`privileged-user`), проксирование по клиентскому сертификату | Программный API (HTTPS/WebSocket/stream) | Привилегированный (interactive VM access) | Внешний | Интерактивные потоки к гостевой ВМ |
| **ValidatingWebhook CRD и `moduleconfigs`** | virtualization-controller (вебхук `0.0.0.0:9443`, Service `:443`) | модульный | Да — admission-валидация (`failurePolicy: Fail` по умолчанию) | Программный API (HTTPS AdmissionReview v1) | Kubernetes API admission | Внутренний | Спецификации VM/VD/VI/CVI/VMBDA/VMClass/VMOP/ModuleConfig |
| **MutatingWebhook `virtualmachines` (defaulter)** | virtualization-controller | модульный | Да — дефолтинг | Программный API (HTTPS AdmissionReview v1) | Kubernetes API admission | Внутренний | Спецификация VirtualMachine |
| **ValidatingAdmissionPolicy внутренних CRD/USB** | Kubernetes admission + модуль | модульный (`templates/admission-policy.yaml`) | Да — `failurePolicy: Fail`, ограничение доступа к `*.internal.virtualization.deckhouse.io` системными SA | Программный API admission (CEL) | Привилегированный системный | Внутренний | Запросы CREATE/UPDATE/DELETE внутренних CRD и usbdevices |
| **CRD виртуализации (VM, VD, VI, CVI, VMBDA, VMClass, snapshots, IP/MAC, USB)** | virtualization-controller + Kubernetes API | модульный (`crds/*`, `api/core/v1alpha2`) | RBAC namespace + admission-валидация | Программный API | Ограниченный RBAC namespace | Внутренний/внешний через Kubernetes API | Произвольные spec, ссылки на образы/диски, cloud-init, sysprep refs |
| **cloud-init / sysprep / provisioning данные в spec ВМ** | virtualization-controller / virt-launcher | модульный | — (валидация ограничена схемой) | Конфигурационный | Ограниченный RBAC namespace | Внутренний | Пользовательские provisioning-данные, передаваемые в гостя |
| **DVCR upload-proxy (приём образов)** | dvcr-uploader | модульный | Да — TLS, client-cert auth | Программный HTTP(S) | Публичный/ограниченный при ingress | Внешний | Загружаемые пользователем образы ВМ |
| **DVCR registry endpoint (push/pull v2)** | DVCR (docker/distribution) | v2.8.3 | Да — basic-auth (htpasswd) | Программный HTTP(S) registry v2 | Ограниченный (внутрикластерный) | Внутренний | Слои/манифесты образов |
| **Импорт образов из внешних источников** | dvcr-importer / CDI importer | CDI v1.60.3; qemu-img/nbdkit | — (обработка недоверенного контента) | Программный (HTTP/registry) | Ограниченный | Внешний | qcow2/raw/iso образы из недоверенных источников |
| **Учётные данные DVCR (`dvcr-secrets`, `dvcr-dockercfg-rw`)** | DVCR / Secret | модульный | — (объект защиты) | Секрет | Привилегированный | Внутренний | passwordRW, htpasswd, salt |
| **`ModuleConfig` virtualization (settings)** | virtualization-controller / Helm | модульный (`openapi/config-values.yaml`) | Да — ValidatingWebhook на UPDATE moduleconfig | Конфигурационный | Привилегированный (оператор DVP) | Внутренний | virtualMachineCIDRs, dvcr storage (PVC), https mode, featureGates, logLevel |
| **TLS-секреты сервисов и CA модуля (`virtualization-ca`, `*-tls`, `*-proxy-tls`)** | controller/api/audit/dvcr | модульный | Да — TLS-инфраструктура | Секрет | Привилегированный | Внутренний | CA, cert, private key |
| **Метрики через kube-rbac-proxy (`:8082` и др.)** | controller/dvcr/cdi-operator | модульный | Да — SubjectAccessReview | Программный HTTPS | Ограниченный RBAC | Внутренний | Метрики с метками namespace/vm/ресурсов |
| **virt-handler hostNetwork порты `4100`/`4101`/`4135-4199`** | virt-handler (DaemonSet) | KubeVirt v1.6.2 | — (привилегированный node-agent) | Сетевой/hostNetwork+hostPID | Привилегированный | Внешний/host | Метрики, console stream, трафик live-миграции |
| **virt-launcher: `/dev/kvm`, libvirt socket, QEMU monitor** | virt-launcher (Pod) | QEMU v9.2.0, libvirt v10.9.0 | seccomp, SELinux, ограниченный whitelist драйверов | Локальный/привилегированный | Привилегированный (root в Pod) | Внутренний/host | Гостевая ОС: ввод-вывод устройств, virtio, файлы дисков |
| **OVMF/edk2 firmware ВМ** | virt-launcher / edk2 | edk2 (commit-pinned) | Да — UEFI Secure Boot, revocation list | Локальный | Привилегированный | Внутренний | Boot-образ, NVRAM-переменные гостя |
| **vm-route-forge route table `1490`, eBPF; health `127.0.0.1:4105`, pprof `:4106`** | vm-route-forge (DaemonSet) | модульный | — (привилегированный, `NET_ADMIN`) | Привилегированный host network interface | Привилегированный | Внутренний/host | Маршруты к ВМ, eBPF-программы, pprof (debug) |
| **virtualization-dra: kubelet plugin gRPC (UNIX socket), `usbipd :4280`, health `:4107`** | virtualization-dra (DaemonSet) | модульный (go-usbip) | — (привилегированный, `SYS_MODULE`) | Привилегированный/программный gRPC | Привилегированный | Внутренний/host | Регистрация плагина, проброс USB, загрузка kernel-модулей |
| **VirtualMachineIPAddress / MAC выделение из `virtualMachineCIDRs`** | virtualization-controller | модульный | — | Программный API | Ограниченный RBAC | Внутренний | Запросы статических IP/MAC; пересечение с подсетью pod/node при мисконфигурации |
| **NodeGroupConfiguration скрипты (selinux, detect-kvm, detect-containerd, aio-max-nr)** | модуль / bashible | модульный | Частично — SELinux-политики | Привилегированный host (bashible) | Привилегированный сборочный/узловой | Внутренний/host | Скрипты, исполняемые на узлах (sysctl, dnf install, node labels, reboot flag) |
| **RBAC роли (user/editor/privileged-user/admin/cluster-editor; rbacv2 manage/use)** | модуль RBAC | модульный (`user-authz-cluster-roles.yaml`, `rbacv2/*`) | Да — разграничение доступа | Kubernetes API | Привилегированный/ограниченный | Внутренний | Назначения ролей; `view_secrets` (чтение всех Secret), `nodes/proxy` verbs `*` |
| **virtualization-audit endpoint (`--secure-port=8443`)** | virtualization-audit (EE) | модульный | Да — TLS, ФБ аудита | Программный HTTPS | Ограниченный | Внутренний | События аудита |
| **`d8` CLI (console/vnc/ssh/scp/portforward/collectdebuginfo)** | src/cli | модульный | Да — через API authn/authz | Клиентский/программный | Ограниченный/привилегированный | Внешний | Команды и потоки к ВМ |
| **Registry/source clone в сборке (`SOURCE_REPO`, `GOPROXY`, `PACKAGE_CLONE_REPO`)** | werf build flow | werf-pipeline | — (вне runtime) | Supply chain | Привилегированный сборочный | Внешний/сборочный | Исходный код KubeVirt/CDI/QEMU/libvirt/edk2/distribution, Go-модули, патчи, образы |
| **Локальные патчи KubeVirt/CDI/QEMU/libvirt** | сборочные образы (`*-artifact`) | commit/tag-pinned | Hardening (seccomp/SELinux/whitelist драйверов) | Supply chain/local execution | Привилегированный сборочный | Внутренний | Патчи к upstream-проектам |

---

<div class="page"></div>

## 4. Идентификация угроз

Для каждого элемента поверхности атаки (раздел 3) угрозы классифицированы по модели STRIDE и сопоставлены с угрозами БДУ ФСТЭК России (УБИ.1–УБИ.11, `Угрозы.csv`). Источник угрозы и потенциал нарушителя определены по категориям субъектов (раздел 1). Нарушаемые свойства: К — конфиденциальность, Ц — целостность, Д — доступность.

<div class="tm-table tm-table-threats"></div>

| Компонент | Элемент поверхности | STRIDE | БДУ | Название угрозы | Источник | Потенциал | Свойства (К/Ц/Д) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **virt-launcher / QEMU/KVM/libvirt** | `/dev/kvm`, virtio/устройства, QEMU monitor (гостевая ОС) | Elevation of Privilege | УБИ.2 | Угроза несанкционированного доступа | Гостевая ОС/нарушитель внутри ВМ | Высокий | К, Ц, Д |
| **virt-launcher / QEMU** | Эмуляция устройств, обработка образа диска | Tampering | УБИ.3 | Угроза несанкционированной модификации (искажения) | Гостевая ОС/нарушитель внутри ВМ | Высокий | Ц |
| **virt-launcher** | Ресурсы CPU/RAM/диск ВМ | Denial of Service | УБИ.8 | Угроза нарушения функционирования (работоспособности) | Гостевая ОС/нарушитель внутри ВМ | Средний | Д |
| **virt-launcher** | Нецелевое использование вычислительных ресурсов (майнинг и т.п.) | DoS / Misuse | УБИ.7 | Угроза ненадлежащего (нецелевого) использования | Пользователь Kubernetes/гость | Средний | Д |
| **virtualization-api** | Агрегированный API, subresources console/vnc/portforward | Spoofing | УБИ.4 | Угроза несанкционированной подмены | Пользователь Kubernetes / внешний нарушитель | Средний | К, Ц |
| **virtualization-api** | console/vnc/portforward к чужой ВМ | Elevation of Privilege | УБИ.2 | Угроза несанкционированного доступа | Пользователь Kubernetes namespace | Средний/Высокий | К, Ц |
| **virtualization-api** | proxy-client-cert к virt-api | Information Disclosure | УБИ.1 | Угроза утечки информации | Внутренний нарушитель | Средний | К |
| **virtualization-api** | Поток API (большое число запросов/stream) | Denial of Service | УБИ.6 | Угроза отказа в обслуживании | Пользователь Kubernetes / внешний нарушитель | Низкий/Средний | Д |
| **virtualization-controller** | Спецификации CRD (VM/VD/VI/VMBDA), cloud-init/sysprep | Tampering | УБИ.3 | Угроза несанкционированной модификации (искажения) | Пользователь Kubernetes namespace | Средний | Ц |
| **virtualization-controller** | ValidatingWebhook (`failurePolicy: Fail`) недоступность | Denial of Service | УБИ.6 | Угроза отказа в обслуживании | Пользователь Kubernetes / внутренний нарушитель | Средний | Д |
| **virtualization-controller** | Обход/ослабление admission-валидации CRD | Tampering | УБИ.3 | Угроза несанкционированной модификации (искажения) | Пользователь Kubernetes namespace | Средний/Высокий | Ц |
| **virtualization-controller** | RBAC list/watch Secret, доступ к ServiceAccount-токену | Elevation of Privilege | УБИ.2 | Угроза несанкционированного доступа | Внутренний нарушитель | Средний/Высокий | К, Ц |
| **DVCR registry / dvcr-uploader** | Загрузка/push образов, basic-auth | Tampering | УБИ.3 | Угроза несанкционированной модификации (искажения) | Пользователь Kubernetes / внешний нарушитель | Средний | Ц |
| **dvcr-importer / CDI importer** | Импорт образа из недоверенного источника | Tampering | УБИ.9 | Угроза получения информационных ресурсов из недоверенного или скомпрометированного источника | Внешний источник/нарушитель | Средний/Высокий | Ц, Д |
| **DVCR** | Учётные данные DVCR (htpasswd, passwordRW, salt) | Information Disclosure | УБИ.1 | Угроза утечки информации | Внутренний нарушитель | Средний/Высокий | К |
| **DVCR storage** | Образы/диски ВМ на PVC | Information Disclosure | УБИ.2 | Угроза несанкционированного доступа | Внутренний нарушитель | Средний | К |
| **DVCR garbage collection** | Удаление образов по расписанию/некорректно | Denial of Service | УБИ.5 | Угроза удаления информационных ресурсов | Внутренний нарушитель / ошибка логики | Средний | Д, Ц |
| **TLS/CA модуля** | `virtualization-ca`, `*-tls`, `*-proxy-tls` | Information Disclosure | УБИ.1 | Угроза утечки информации | Внутренний нарушитель | Высокий | К |
| **ModuleConfig** | settings (CIDR, dvcr, featureGates, logLevel) | Tampering | УБИ.3 | Угроза несанкционированной модификации (искажения) | Оператор DVP / внутренний нарушитель | Средний | Ц, Д |
| **virt-handler (DaemonSet)** | hostNetwork+hostPID, privileged, hostPath к kubelet | Elevation of Privilege | УБИ.2 | Угроза несанкционированного доступа | Внутренний нарушитель с доступом в Pod/ноду | Высокий | К, Ц, Д |
| **virt-handler** | Трафик live-миграции (`4135-4199`) | Information Disclosure | УБИ.1 | Угроза утечки информации | Сетевой/внутренний нарушитель | Средний | К |
| **vm-route-forge (DaemonSet)** | route table `1490`, eBPF, `NET_ADMIN`, hostNetwork | Tampering | УБИ.3 | Угроза несанкционированной модификации (искажения) | Внутренний нарушитель | Высокий | Ц, Д |
| **vm-route-forge** | Маршрутизация трафика ВМ / нарушение | Denial of Service | УБИ.8 | Угроза нарушения функционирования (работоспособности) | Внутренний нарушитель | Средний | Д |
| **vm-route-forge** | pprof `:4106` (debug) | Information Disclosure | УБИ.1 | Угроза утечки информации | Внутренний нарушитель | Низкий/Средний | К |
| **virtualization-dra (DaemonSet)** | privileged, `SYS_MODULE`, hostPath `/lib/modules`,`/sys` | Elevation of Privilege | УБИ.2 | Угроза несанкционированного доступа | Внутренний нарушитель | Высокий | К, Ц, Д |
| **virtualization-dra-usb** | Проброс USB (`usbipd :4280`) | Tampering | УБИ.3 | Угроза несанкционированной модификации (искажения) | Внутренний нарушитель | Средний | Ц |
| **VirtualMachineIPAddress/MAC** | Выделение IP/MAC из `virtualMachineCIDRs`, подмена адреса | Spoofing | УБИ.4 | Угроза несанкционированной подмены | Пользователь Kubernetes namespace | Средний | Ц |
| **RBAC** | `view_secrets` (все Secret), `nodes/proxy` verbs `*` | Elevation of Privilege | УБИ.2 | Угроза несанкционированного доступа | Пользователь Kubernetes (при широкой агрегации) | Средний/Высокий | К, Ц |
| **virtualization-audit** | Подавление/обход аудита | Repudiation | УБИ.3 | Угроза несанкционированной модификации (искажения) | Внутренний нарушитель | Средний | Ц |
| **Метрики/логи** | Метрики через kube-rbac-proxy, access logs, `logLevel: debug` | Information Disclosure | УБИ.11 | Угроза несанкционированного массового сбора информации | Внутренний нарушитель | Низкий/Средний | К |
| **NodeGroupConfiguration** | Скрипты bashible на узлах (sysctl, dnf, selinux, reboot) | Tampering | УБИ.3 | Угроза несанкционированной модификации (искажения) | Внутренний нарушитель сборочной/узловой среды | Высокий | Ц, Д |
| **Build/update flow** | `SOURCE_REPO`, `GOPROXY`, registry, патчи KubeVirt/CDI/QEMU/libvirt/edk2 | Tampering | УБИ.9 | Угроза получения информационных ресурсов из недоверенного или скомпрометированного источника | Внешний поставщик/внутренний нарушитель | Высокий | К, Ц, Д |
| **Build/update flow** | Внедрение закладки в сборку образа | Tampering | УБИ.3 | Угроза несанкционированной модификации (искажения) | Внутренний нарушитель | Высокий | Ц |
| **virtualization-api / DVCR / CRD** | Сетевое сканирование/идентификация сервисов и версий | Information Disclosure | УБИ.11 | Угроза несанкционированного массового сбора информации | Внутренний/внешний нарушитель | Низкий | К |
| **ВМ как ресурс** | Использование ВМ для рассылки/проксирования противоправного трафика | Misuse | УБИ.10 | Угроза распространения противоправной информации | Пользователь/нарушитель внутри ВМ | Низкий/Средний | Ц, Д |

<div class="tm-section-gap"></div>

**Покрытие STRIDE:**

<div class="tm-table tm-table-stride-summary"></div>

| STRIDE | Покрытые элементы | Вывод |
| --- | --- | --- |
| Spoofing | API/console proxy, VM IP/MAC, источник образов | Риск связан с подменой субъекта доступа к ВМ, подменой адреса ВМ и подменой внешнего источника образов. |
| Tampering | CRD specs, admission, образы DVCR, route table, iptables/eBPF, NodeGroupConfiguration, build artifacts | Риск связан с изменением спецификаций ВМ, образов, сетевой маршрутизации, узловой конфигурации и цепочки поставки. |
| Repudiation | Аудит (EE), действия через Kubernetes API, CI/CD | Риск связан с недостаточным журналированием действий: `virtualization-audit` доступен только в EE и по умолчанию выключен, аудит доступа к Secret/DVCR и действий через Kubernetes API ограничен. |
| Information Disclosure | TLS/CA-секреты, DVCR-учётки, метрики, логи, pprof, live-migration трафик | Риск связан с раскрытием секретов, образов/дисков ВМ и внутренней топологии. |
| Denial of Service | API, admission webhook (`Fail`), virt-launcher ресурсы, vm-route-forge, DVCR GC | Риск связан с публичными/полупубличными точками входа, блокирующей admission-валидацией и привилегированными сетевыми компонентами. |
| Elevation of Privilege | virt-launcher (VM escape), привилегированные DaemonSet (virt-handler/vm-route-forge/dra), RBAC, console-доступ | Риск связан с побегом из ВМ, эксплуатацией привилегированных компонентов и преобразованием namespace-доступа в доступ к узлу/кластеру. |

---

<div class="tm-section-gap tm-section-gap-md"></div>

## 5. Моделирование сценариев атак

Сценарии сформированы на основе перечня угроз (раздел 4), архитектурной модели (раздел 2) и поверхности атаки (раздел 3). Для типизации действий использованы тактики ПРИЛОЖЕНИЯ 1 методики и материалы CAPEC/ATT&CK. В соответствии с методикой шифрование данных, передаваемых через loopback внутри/между модулями, не рассматривается как недостаток.

### Сценарий AS-01. Побег из гостевой ВМ на узел (VM escape) через QEMU/KVM/libvirt

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-01 |
| **Связанная угроза** | УБИ.2, УБИ.3, УБИ.8 |
| **Элемент поверхности атаки** | virt-launcher: `/dev/kvm`, эмуляция устройств QEMU v9.2.0, libvirt v10.9.0, virtio |
| **Источник угрозы** | Нарушитель с правами выполнения кода в гостевой ОС ВМ (недоверенный субъект) |
| **Начальный уровень доступа** | Root/администратор внутри гостевой ВМ |
| **Вектор атаки** | Эксплуатация уязвимости эмулятора устройств/гипервизора (device emulation, virtio, escape), обход seccomp/SELinux в `virt-launcher` |
| **Используемая уязвимость** | Потенциальные уязвимости QEMU/KVM/libvirt; `virt-launcher` исполняется от root; SBOM Go-части + нативный слой предоставлены (`tmp/sbom-output/main/virtlauncher/`, 460 нативных артефактов по SHA-256, вкл. qemu-system/qemu-img/libvirt.so), но нативные QEMU/libvirt/edk2 не привязаны к CVE (Trivy `OS: None`), OpenVEX-statements пусты; компенсируется seccomp, SELinux и whitelist блочных драйверов |
| **Краткая последовательность действий** | <ol><li>Нарушитель получает контроль над гостевой ОС.</li><li>Триггерит уязвимость эмуляции устройства/гипервизора.</li><li>Выполняет код в контексте `virt-launcher` (root в Pod).</li><li>Через привилегии Pod/hostPath атакует узел и соседние ВМ.</li></ol> |
| **Последствия** | Компрометация узла и соседних ВМ, нарушение изоляции, потенциальный захват кластера. |

### Сценарий AS-02. Несанкционированный доступ к чужой ВМ через console/VNC/portforward

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-02 |
| **Связанная угроза** | УБИ.2, УБИ.4, УБИ.1 |
| **Элемент поверхности атаки** | virtualization-api subresources `virtualmachines/console`, `/vnc`, `/portforward`; RBAC `privileged-user` |
| **Источник угрозы** | Пользователь Kubernetes с избыточными правами или внутренний нарушитель |
| **Начальный уровень доступа** | Аутентифицированный пользователь кластера с доступом к subresources |
| **Вектор атаки** | Использование RBAC-прав на console/vnc/portforward в чужом namespace; эксплуатация ошибок авторизации в агрегированном API |
| **Используемая уязвимость** | Чрезмерно широкие RoleBinding роли `privileged-user`; ошибки delegating authorization; требует уточнения фактических назначений |
| **Краткая последовательность действий** | <ol><li>Нарушитель получает право на subresource.</li><li>Открывает console/VNC к ВМ цели.</li><li>Взаимодействует с гостевой ОС (ввод/перехват экрана).</li><li>Получает доступ к данным/учётным записям внутри ВМ.</li></ol> |
| **Последствия** | Нарушение конфиденциальности и целостности чужих ВМ, компрометация гостевых учётных данных. |

### Сценарий AS-03. Воздействие на ВМ через спецификацию CRD, cloud-init и обход admission

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-03 |
| **Связанная угроза** | УБИ.3, УБИ.2 |
| **Элемент поверхности атаки** | CRD VM/VD/VI/VMBDA, cloud-init/sysprep, ValidatingWebhook controller |
| **Источник угрозы** | Пользователь Kubernetes namespace с правом `create/update` ресурсов ВМ (`editor`) |
| **Начальный уровень доступа** | Ограниченный RBAC в namespace приложения |
| **Вектор атаки** | Создание VM с вредоносными provisioning-данными, привязка чужого диска/образа (VMBDA), эксплуатация недостатков валидации спецификации |
| **Используемая уязвимость** | Недостаточная проверка provisioning-данных/ссылок; зависимость целостности от доступности вебхука; недостатки проверки входных данных (СП.2.1) |
| **Краткая последовательность действий** | <ol><li>Нарушитель создаёт/изменяет VM в своём namespace.</li><li>Внедряет cloud-init/привязку диска/образа.</li><li>Admission пропускает объект (или недоступен).</li><li>Контроллер материализует конфигурацию ВМ.</li></ol> |
| **Последствия** | Изменение поведения ВМ, доступ к чужим дискам/образам, нарушение целостности конфигурации. |

### Сценарий AS-04. Отказ admission-валидации (`failurePolicy: Fail`) → DoS плоскости управления

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-04 |
| **Связанная угроза** | УБИ.6, УБИ.8 |
| **Элемент поверхности атаки** | ValidatingWebhook `virtualization-controller-admission-webhook` (Service `:443`, без явного `failurePolicy` → `Fail`) |
| **Источник угрозы** | Внутренний нарушитель или пользователь, способный нагрузить/уронить контроллер |
| **Начальный уровень доступа** | Возможность воздействовать на доступность `virtualization-controller` (нагрузка, удаление Pod, сетевая изоляция) |
| **Вектор атаки** | Вывод контроллера из строя; при `failurePolicy: Fail` все CREATE/UPDATE целевых CRD и `ModuleConfig` блокируются |
| **Используемая уязвимость** | Жёсткая зависимость операций от доступности вебхук-сервера контроллера |
| **Краткая последовательность действий** | <ol><li>Нарушитель снижает доступность контроллера.</li><li>Admission-запросы завершаются ошибкой.</li><li>Управление ВМ/дисками/образами блокируется.</li></ol> |
| **Последствия** | Отказ в обслуживании плоскости управления виртуализацией; невозможность создавать/изменять ресурсы ВМ. |

### Сценарий AS-05. Компрометация учётных данных DVCR и TLS-секретов модуля

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-05 |
| **Связанная угроза** | УБИ.1, УБИ.2 |
| **Элемент поверхности атаки** | `dvcr-secrets`, `dvcr-dockercfg-rw`, `virtualization-ca`, `*-tls`; RBAC `view_secrets` |
| **Источник угрозы** | Внутренний нарушитель |
| **Начальный уровень доступа** | Доступ к Pod/ServiceAccount-токену либо роль, агрегирующая `view_secrets` (чтение всех Secret) |
| **Вектор атаки** | Чтение Secret через Kubernetes API, mounted volume или широкие RBAC-права |
| **Используемая уязвимость** | Капабилити `view_secrets` (get/list/watch всех `secrets`), широкая видимость секретов контроллером; недостатки настройки прав доступа (СП.2.11) |
| **Краткая последовательность действий** | <ol><li>Нарушитель получает токен/RBAC.</li><li>Читает htpasswd DVCR, TLS private keys, CA.</li><li>Использует их для доступа к образам, MITM или подмены сервисов.</li></ol> |
| **Последствия** | Раскрытие/подмена образов ВМ, MITM внутреннего TLS, компрометация доверия. |

### Сценарий AS-06. Подмена образа ВМ через импорт из недоверенного источника / DVCR

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-06 |
| **Связанная угроза** | УБИ.9, УБИ.4, УБИ.3 |
| **Элемент поверхности атаки** | Полный жизненный цикл образа ВМ: (1) **импорт** — `VirtualImage`/`ClusterVirtualImage.spec.dataSource` типы `HTTP` (`http.url`, pattern `^http[s]?://` — допускает плайн HTTP), `ContainerImage` (внешний registry), `ObjectRef`; CDI v1.60.3 `cdi-importer`/`dvcr-importer`, `dvcr-uploader` (upload-proxy); (2) **конвертация/распаковка** — qemu-img, `nbdkit v1.39.5`, `libnbd v1.23.6`, `libxml2` (qcow2/vmdk/vdi/iso/raw, gz/xz); (3) **хранение** — DVCR (distribution v2.8.3), backend PVC; (4) **раздача** — pull в `virt-launcher` |
| **Источник угрозы** | Внешний источник образов, скомпрометированный mirror/registry, пользователь с правом импорта, сетевой нарушитель (MitM при HTTP/skip-TLS) |
| **Начальный уровень доступа** | Возможность указать источник образа (VI/CVI) или загрузить образ через upload-proxy; либо контроль над внешним источником/каналом доставки |
| **Вектор атаки** | (а) Подмена образа в источнике или MitM (если `http.url` без TLS либо HTTPS с пропуском проверки и без `caBundle`) при **незаполненном опциональном `checksum` (md5/sha256)**; (б) эксплуатация парсинга формата при конвертации (malformed qcow2/vmdk → qemu-img/nbdkit); (в) подмена blob/manifest уже в DVCR (см. AS-05/AS-04) |
| **Используемая уязвимость** | Импорт обрабатывает недоверенный внешний контент; **проверка целостности доступна, но необязательна**: `checksum.md5/sha256` и `caBundle` — опциональные поля схемы CRD; **криптографическая подпись образа (cosign/notation) не проверяется**; недостатки проверки входных данных (СП.2.1); исторически уязвимый класс парсеров qemu-img/nbdkit |
| **Краткая последовательность действий** | <ol><li>Нарушитель публикует/подменяет образ в источнике или перехватывает канал доставки.</li><li>`cdi-importer`/`dvcr-importer` скачивает и (при отсутствии `checksum`) не верифицирует целостность.</li><li>qemu-img/nbdkit конвертирует образ (возможна эксплуатация парсера в Pod импортёра).</li><li>Образ кладётся в DVCR и используется для запуска ВМ.</li><li>ВМ стартует со скомпрометированным содержимым.</li></ol> |
| **Последствия** | Запуск скомпрометированных ВМ, нарушение целостности образов, эксплуатация парсера образов (RCE в Pod импортёра с доступом к DVCR-учёткам), распространение вредоносных образов между ВМ. |

### Сценарий AS-07. Повышение привилегий до узла через привилегированные DaemonSet (virt-handler / vm-route-forge / virtualization-dra)

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-07 |
| **Связанная угроза** | УБИ.2, УБИ.3 |
| **Элемент поверхности атаки** | virt-handler (`privileged`, `hostNetwork`+`hostPID`, hostPath kubelet), vm-route-forge (`NET_ADMIN`, hostNetwork, root), virtualization-dra (`SYS_MODULE`, hostPath `/lib/modules`,`/sys`) |
| **Источник угрозы** | Внутренний нарушитель с возможностью выполнить код в одном из привилегированных Pod |
| **Начальный уровень доступа** | Доступ в namespace `d8-virtualization` / эксплуатация одного из компонентов (в т.ч. через AS-01/AS-06) |
| **Вектор атаки** | Использование hostPath RW, `hostPID`, `SYS_MODULE` (загрузка kernel-модуля), доступа к kubelet-сокетам для выхода на узел |
| **Используемая уязвимость** | Привилегированная конфигурация DaemonSet по необходимости (node-agent, маршрутизация, DRA); компенсируется ограничением доступа в namespace |
| **Краткая последовательность действий** | <ol><li>Нарушитель выполняет код в привилегированном Pod.</li><li>Через hostPath/hostPID/`SYS_MODULE` получает доступ к ФС/процессам/ядру узла.</li><li>Закрепляется на узле и эскалирует на кластер.</li></ol> |
| **Последствия** | Полная компрометация узла, соседних Pod/ВМ и потенциально кластера. |

### Сценарий AS-08. Нарушение сетевой маршрутизации/изоляции ВМ через vm-route-forge

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-08 |
| **Связанная угроза** | УБИ.3, УБИ.8, УБИ.4 |
| **Элемент поверхности атаки** | vm-route-forge: route table `1490`, eBPF, `NET_ADMIN`, hostNetwork; `virtualMachineCIDRs` |
| **Источник угрозы** | Внутренний нарушитель; пользователь с правом задавать VM IP |
| **Начальный уровень доступа** | Воздействие на vm-route-forge или на VirtualMachineIPAddress/CIDR-конфигурацию |
| **Вектор атаки** | Изменение маршрутов/eBPF, подмена IP/MAC ВМ, пересечение `virtualMachineCIDRs` с подсетью pod/node |
| **Используемая уязвимость** | Привилегии `NET_ADMIN`/hostNetwork; зависимость изоляции от корректности CIDR (предупреждение в `config-values.yaml`) |
| **Краткая последовательность действий** | <ol><li>Нарушитель влияет на маршруты или адресацию ВМ.</li><li>Трафик ВМ перенаправляется/блокируется.</li><li>Нарушается доступность или изоляция сети.</li></ol> |
| **Последствия** | Отказ в обслуживании сети ВМ, перехват/подмена трафика, нарушение сетевой изоляции. |

### Сценарий AS-09. Компрометация цепочки поставки образов (KubeVirt/CDI/QEMU/libvirt/edk2/DVCR)

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-09 |
| **Связанная угроза** | УБИ.9, УБИ.3, УБИ.2 |
| **Элемент поверхности атаки** | `SOURCE_REPO`, `GOPROXY`, `PACKAGE_CLONE_REPO`, registry, локальные патчи, `werf.inc.yaml`, `werf-giterminism.yaml` |
| **Источник угрозы** | Внешний поставщик, скомпрометированный репозиторий/зеркало, внутренний нарушитель сборочной среды |
| **Начальный уровень доступа** | Доступ к source-репозиториям, dependency mirror, build secrets или registry |
| **Вектор атаки** | Подмена исходного кода/tag/commit, Go-модуля, базового образа, патча; внедрение закладки (СП.4.11, СП.5.1) |
| **Используемая уязвимость** | Большая цепочка форков и нативных зависимостей (QEMU/libvirt/nbdkit/distribution); SBOM Go-части + нативный слой по 21 образу предоставлен (`tmp/sbom-output/main/`, нативные бинарники зафиксированы по SHA-256), но нативный слой не привязан к rpm/apk-пакетам, OpenVEX-statements пусты — supply-chain CVE по нативу остаются непривязанными |
| **Краткая последовательность действий** | <ol><li>Нарушитель внедряет изменение в зависимость/патч.</li><li>Изменение попадает в build artifact.</li><li>Образ публикуется и разворачивается.</li><li>Код исполняется в контексте virt-launcher/контроллера/DaemonSet.</li></ol> |
| **Последствия** | Полная компрометация виртуализационного runtime, секретов, образов и узлов. |

### Сценарий AS-10. Раскрытие информации и подавление аудита

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-10 |
| **Связанная угроза** | УБИ.1, УБИ.11, УБИ.3 (Repudiation) |
| **Элемент поверхности атаки** | Метрики через kube-rbac-proxy, access logs, `logLevel: debug`, vm-route-forge pprof `:4106`, virtualization-audit (EE) |
| **Источник угрозы** | Внутренний нарушитель |
| **Начальный уровень доступа** | Доступ к Prometheus/метрикам, логам Pod, debug-endpoint или к управлению `audit.enabled` |
| **Вектор атаки** | Массовый сбор меток метрик (namespace/vm/ресурсы), чтение логов в debug-режиме, обращение к pprof, отключение/обход аудита |
| **Используемая уязвимость** | Метрики/логи содержат внутреннюю топологию; pprof и debug-режимы при доступности раскрывают данные; аудит EE может быть выключен (`audit.enabled: false` по умолчанию) |
| **Краткая последовательность действий** | <ol><li>Нарушитель получает доступ к мониторингу/логам/debug.</li><li>Собирает карту ресурсов и ВМ.</li><li>При отключённом аудите скрывает действия.</li></ol> |
| **Последствия** | Раскрытие внутренней структуры, повышение эффективности последующих атак, невозможность расследования. |

### Сценарий AS-11. Атака на live-migration ВМ через миграционные туннели virt-handler (порты 4135–4199)

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-11 |
| **Связанная угроза** | УБИ.1, УБИ.3, УБИ.8 (Tampering/Information Disclosure/DoS) |
| **Элемент поверхности атаки** | `virt-handler` (hostNetwork): диапазон миграционных туннелей **TCP 4135–4199** на сетевых интерфейсах узла (см. `templates/_hostnetwork_ports.tpl`); `VirtualMachineOperation` (`evict`/`migrate`); `MigrationPolicy`/`migrations.internal.virtualization.deckhouse.io` |
| **Источник угрозы** | Внутренний нарушитель в L3/L4-сегменте узлов (отсутствие NetworkPolicy) либо нарушитель, способный инициировать миграцию через `VMOP` |
| **Начальный уровень доступа** | Сетевой доступ к узлу на портах 4135–4199 (поскольку `virt-handler` слушает на `hostNetwork`, порт открыт на всех интерфейсах узла), или RBAC на создание `VirtualMachineOperation` |
| **Вектор атаки** | (а) MitM/перехват состояния миграции (RAM/устройства ВМ передаются между узлами по миграционному туннелю); (б) массовая принудительная миграция через `VMOP migrate` для перегрузки сети/узлов (DoS); (в) попытка инъекции в поток миграции при недостаточной взаимной аутентификации туннеля |
| **Используемая уязвимость** | Миграционные порты экспонированы на `hostNetwork` без сетевого периметра (нет NetworkPolicy/CiliumNetworkPolicy); конфиденциальность/целостность миграции зависят от TLS-настроек KubeVirt и сегментации сети узлов; СП.2.11 (недостатки разграничения доступа), СП.2.1 |
| **Краткая последовательность действий** | <ol><li>Нарушитель получает позицию в сети узлов или право `VMOP`.</li><li>Триггерит/перехватывает live-migration.</li><li>Перехватывает образ памяти ВМ в транзите либо вызывает каскадные миграции.</li><li>Извлекает секреты из дампа RAM или вызывает отказ узла/сети.</li></ol> |
| **Последствия** | Раскрытие содержимого памяти ВМ (ключи, токены) в транзите, нарушение доступности узлов и сети при миграционном шторме, потенциальное нарушение целостности мигрируемой ВМ. |

### Сценарий AS-12. Эскалация через USB-passthrough DRA и загрузку kernel-модулей usbip на всех узлах

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-12 |
| **Связанная угроза** | УБИ.2, УБИ.3 (Elevation of Privilege/Tampering) |
| **Элемент поверхности атаки** | `virtualization-dra` DaemonSet: `privileged: true`, capability `SYS_MODULE`, hostPath RW к `/lib/modules`,`/sys`,`/var/run`,`/var/lib/kubelet/plugins{,_registry}` (`templates/virtualization-dra/daemonset.yaml`); USB/IP daemon на **TCP 4280** (hostNetwork); `NodeGroupConfiguration virtualization-install-usbip-modules` (`bundles: '*'`, `nodeGroups: '*'`) загружает `usbip-core.ko`/`usbip-host.ko`/`vhci-hcd.ko` на **все** узлы; `DeviceClass usb-devices.virtualization.deckhouse.io` (driver `virtualization-usb`); `nodeusbdevice.internal.virtualization.deckhouse.io` |
| **Источник угрозы** | Внутренний нарушитель с возможностью выполнить код в Pod `virtualization-dra` (в т.ч. через AS-09) либо нарушитель, эксплуатирующий usbipd на порту 4280 по сети узла |
| **Начальный уровень доступа** | Исполнение кода в привилегированном DRA-Pod, либо сетевой доступ к usbipd `:4280`, либо право на `VirtualMachine` с `usb`-устройством через DRA-claim |
| **Вектор атаки** | (а) Использование `SYS_MODULE` для загрузки произвольного kernel-модуля → выполнение кода в ядре узла; (б) эксплуатация USB/IP-стека ядра (`vhci-hcd`/`usbip-host`) — исторически уязвимый класс (use-after-free, OOB в драйверах USB) — для LPE на узле; (в) подключение вредоносного «виртуального» USB-устройства к чужой ВМ через DRA |
| **Используемая уязвимость** | Привилегированный DaemonSet с `SYS_MODULE` (полный root-эквивалент на узле); принудительная установка usbip kernel-модулей на ВСЕ узлы расширяет attack surface ядра даже там, где USB-passthrough не используется; usbipd-порт открыт на `hostNetwork` без NetworkPolicy |
| **Краткая последовательность действий** | <ol><li>Нарушитель исполняет код в DRA-Pod или достигает usbipd `:4280`.</li><li>Через `SYS_MODULE`/USB-IP-стек выполняет код в ядре узла.</li><li>Закрепляется на узле; через hostPath kubelet-plugins эскалирует на соседние Pod/ВМ.</li></ol> |
| **Последствия** | Полная компрометация узла (kernel-level), компрометация всех ВМ узла, расширение поверхности атаки ядра на узлах без потребности в USB-passthrough. |

### Сценарий AS-13. Инъекция через cloud-init/sysprep provisioning-секреты ВМ

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-13 |
| **Связанная угроза** | УБИ.2, УБИ.3, УБИ.1 (Elevation of Privilege/Tampering/Spoofing) |
| **Элемент поверхности атаки** | `VirtualMachine.spec.provisioning` (cloud-init `userData`/`networkData`, sysprep); Kubernetes `Secret` с provisioning-данными; capability `view_secrets`, агрегированный в роль `user`; ValidatingWebhook контроллера (без явного `failurePolicy`) |
| **Источник угрозы** | Пользователь с ролью `editor`/`user` в tenant-namespace, либо нарушитель, получивший чтение/запись provisioning-секретов |
| **Начальный уровень доступа** | RBAC на `create/update` `VirtualMachine` и/или на `Secret` в namespace (роль `user` даёт `get/list/watch secrets`) |
| **Вектор атаки** | (а) Внедрение вредоносного cloud-init (`runcmd`, `write_files`, добавление SSH-ключей/пользователей) в спецификацию ВМ или в referenced Secret; (б) подмена существующего provisioning-секрета чужой ВМ (доступного через `view_secrets`) → выполнение кода при следующей перезагрузке/пересоздании ВМ; (в) кража userData с встроенными секретами |
| **Используемая уязвимость** | `view_secrets` (чтение всех секретов) в базовой роли `user`; admission не проверяет семантику cloud-init (СП.2.1); зависимость целостности от доступности вебхука |
| **Краткая последовательность действий** | <ol><li>Нарушитель читает или подменяет provisioning-Secret цели (через `view_secrets`/`update`).</li><li>Внедряет команды/ключи в cloud-init/sysprep.</li><li>Целевая ВМ применяет provisioning при старте.</li><li>Нарушитель получает доступ к гостевой ОС чужой ВМ.</li></ol> |
| **Последствия** | Выполнение произвольного кода/получение доступа в чужих ВМ, кража встроенных в userData секретов, закрепление через инъекцию SSH-ключей. |

### Сценарий AS-14. Нарушение целостности через многоверсионность `VirtualMachineClass` и отсутствие conversion-webhook

<div class="tm-table tm-table-scenario"></div>

| Параметр | Значение |
| --- | --- |
| **ID сценария** | AS-14 |
| **Связанная угроза** | УБИ.3, УБИ.8 (Tampering/DoS) |
| **Элемент поверхности атаки** | CRD `VirtualMachineClass` (две версии: storage-версия + served-версия, `crds/virtualmachineclasses.yaml`); внутренние CRD KubeVirt/VM с `conversion.strategy: None` (`crds/embedded/kubevirt.yaml`, `crds/embedded/virtualmachines.yaml`); ValidatingWebhook контроллера для VMClass |
| **Источник угрозы** | Пользователь/нарушитель с правом `create/update` `VirtualMachineClass` (cluster-scoped ресурс — влияет на размещение всех ВМ); внутренний нарушитель |
| **Начальный уровень доступа** | RBAC на запись `VirtualMachineClass` (роль `manage`/cluster-admin модуля) |
| **Вектор атаки** | (а) Манипуляция `VirtualMachineClass` (CPU-модель, `nodeSelector`/`tolerations`, `sizingPolicies`) для принудительного размещения ВМ на узле-цели (co-residency для AS-01/AS-11) или обхода ограничений; (б) эксплуатация рассогласования представления объекта между двумя версиями CRD при `conversion.strategy: None` (round-trip без conversion-webhook может приводить к потере/искажению полей, если разные клиенты используют разные версии) |
| **Используемая уязвимость** | Отсутствие conversion-webhook для multi-version CRD (полагается на структурную совместимость схем); cluster-scope `VirtualMachineClass` влияет на размещение всех ВМ; недостатки проверки входных данных при изменении класса (СП.2.1) |
| **Краткая последовательность действий** | <ol><li>Нарушитель изменяет `VirtualMachineClass`.</li><li>Контроллер применяет новую политику размещения/sizing ко всем ВМ, использующим класс.</li><li>ВМ цели переезжают на подконтрольный узел / получают изменённую конфигурацию CPU.</li><li>Создаются условия для co-residency-атак или отказа планирования.</li></ol> |
| **Последствия** | Принудительное co-residency для последующего VM escape (AS-01) или перехвата миграции (AS-11), отказ в планировании ВМ (DoS), нарушение целостности конфигурации класса. |

---

<div class="tm-section-gap tm-section-gap-md"></div>

## 6. Оценка актуальности и формирование модели угроз

Оценка выполнена по критериям методики: угроза признаётся актуальной при одновременном наличии источника угрозы, реализуемого сценария, уязвимости и условий эксплуатации. Категории (Критический/Высокий/Средний/Низкий) присвоены по правилам раздела 6 методики.

<div class="tm-table tm-table-actuality"></div>

| ID угрозы | Сценарий | Уязвимость | Реализуемость | Ущерб | Категория | Решение | Обоснование |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **УБИ.2, УБИ.3, УБИ.8** | AS-01 | Да | Низкая/Средняя | Критический | Критический | **Актуальна** | `virt-launcher` исполняет недоверенную гостевую ОС от root; VM escape ведёт к компрометации узла. Компенсируется seccomp/SELinux/whitelist драйверов; SBOM Go-части + нативный слой доступны (`tmp/sbom-output/main/virtlauncher/`, 460 нативных артефактов по SHA-256), но нативные QEMU/libvirt не привязаны к CVE и OpenVEX-statements пусты. |
| **УБИ.2, УБИ.4, УБИ.1** | AS-02 | Да | Средняя | Высокий | Высокий | **Актуальна** | Доступ к console/VNC/portforward определяется RBAC `privileged-user`; при широких RoleBinding возможен доступ к чужим ВМ. |
| **УБИ.3, УБИ.2** | AS-03 | Да | Средняя | Высокий | Высокий | **Актуальна** | Пользователь namespace создаёт VM с provisioning-данными и привязкой дисков; безопасность зависит от полноты admission-валидации. |
| **УБИ.6, УБИ.8** | AS-04 | Да | Средняя | Средний/Высокий | Высокий | **Актуальна** | ValidatingWebhook без явного `failurePolicy` → `Fail`; недоступность контроллера блокирует управление ВМ. |
| **УБИ.1, УБИ.2** | AS-05 | Да | Средняя | Критический | Критический | **Актуальна** | Капабилити `view_secrets` и широкая видимость секретов; компрометация даёт htpasswd DVCR и TLS-ключи. |
| **УБИ.9, УБИ.4, УБИ.3** | AS-06 | Да | Средняя | Высокий | Высокий | **Актуальна** | Импорт обрабатывает недоверенные внешние образы через qemu-img/nbdkit без обязательной проверки подписи источника. |
| **УБИ.2, УБИ.3** | AS-07 | Да | Низкая/Средняя | Критический | Критический | **Актуальна** | virt-handler/vm-route-forge/virtualization-dra привилегированы (hostNetwork, hostPID, `SYS_MODULE`, hostPath); эксплуатация ведёт к захвату узла. |
| **УБИ.3, УБИ.8, УБИ.4** | AS-08 | Да | Низкая/Средняя | Высокий | Высокий | **Актуальна** | vm-route-forge имеет `NET_ADMIN`/hostNetwork; изоляция зависит от корректности `virtualMachineCIDRs`. |
| **УБИ.9, УБИ.3, УБИ.2** | AS-09 | Да | Средняя | Критический | Критический | **Актуальна** | Большая цепочка форков и нативных зависимостей; SBOM по 21 образу (`tmp/sbom-output/main/`) покрывает Go-биндинги (включая `stdlib 1.25.10`, `kubevirt.io/containerized-data-importer-api v1.63.1`) и нативный слой по SHA-256 (`native-artifacts.tsv`); CVE-привязка нативных ОС-пакетов и OpenVEX-statements — открыты. |
| **УБИ.1, УБИ.11, УБИ.3** | AS-10 | Да | Средняя | Средний | Средний | **Актуальна** | Метрики/логи содержат внутреннюю топологию; pprof присутствует в бинарях (например `vm-route-forge :4106`), аудит по умолчанию выключен (`audit.enabled: false`) — раскрытие диагностики и отсутствие журналирования действий. |
| **УБИ.1, УБИ.3, УБИ.8** | AS-11 | Да | Средняя | Высокий | Высокий | **Актуальна** | Миграционные туннели 4135–4199 экспонированы на `hostNetwork` без NetworkPolicy; перехват памяти ВМ или миграционный шторм. |
| **УБИ.2, УБИ.3** | AS-12 | Да | Низкая/Средняя | Критический | Критический | **Актуальна** | DRA-DaemonSet `privileged`+`SYS_MODULE`; usbip kernel-модули устанавливаются на все узлы; usbipd `:4280` на hostNetwork. |
| **УБИ.2, УБИ.3, УБИ.1** | AS-13 | Да | Средняя | Высокий | Высокий | **Актуальна** | `view_secrets` в роли `user` даёт чтение/подмену cloud-init/sysprep секретов; admission не проверяет семантику provisioning. |
| **УБИ.3, УБИ.8** | AS-14 | Да | Низкая/Средняя | Средний | Средний | **Актуальна** | cluster-scope `VirtualMachineClass` влияет на размещение всех ВМ; multi-version CRD без conversion-webhook (`strategy: None`); реализуемость определяется RBAC на запись класса. |

<div class="tm-section-gap tm-section-gap-md"></div>

### Итоговая модель актуальных угроз

<div class="tm-table tm-table-threat-model"></div>

| ID | Угроза | Актуальность | Основные компоненты | Приоритет нейтрализации |
| --- | --- | --- | --- | --- |
| TM-01 | Побег из гостевой ВМ на узел (VM escape) | Актуальна | virt-launcher, QEMU, KVM, libvirt | Критический |
| TM-02 | Несанкционированный доступ к чужой ВМ (console/VNC/portforward) | Актуальна | virtualization-api, virt-api, RBAC | Высокий |
| TM-03 | Небезопасное воздействие на ВМ через CRD/cloud-init и обход admission | Актуальна | virtualization-controller, admission, Kubernetes API | Высокий |
| TM-04 | Отказ admission-валидации → DoS плоскости управления | Актуальна | virtualization-controller, ValidatingWebhook | Высокий |
| TM-05 | Компрометация секретов DVCR и TLS/CA модуля | Актуальна | DVCR, Secrets, RBAC `view_secrets` | Критический |
| TM-06 | Подмена образа ВМ при импорте из недоверенного источника | Актуальна | dvcr-importer, CDI importer, dvcr-uploader, DVCR | Высокий |
| TM-07 | Повышение привилегий до узла через привилегированные DaemonSet | Актуальна | virt-handler, vm-route-forge, virtualization-dra | Критический |
| TM-08 | Нарушение сетевой маршрутизации/изоляции ВМ | Актуальна | vm-route-forge, VM IP/MAC, virtualMachineCIDRs | Высокий |
| TM-09 | Компрометация цепочки поставки образов | Актуальна | werf build, source repos, registry, патчи | Критический |
| TM-10 | Раскрытие метрик/логов/диагностики и подавление аудита | Актуальна | kube-rbac-proxy, логи, pprof, virtualization-audit | Средний |

<div class="tm-section-gap tm-section-gap-md"></div>

### Меры по нейтрализации

<div class="tm-table tm-table-mitigations"></div>

| Угроза | Приоритет | Рекомендуемые меры |
| --- | --- | --- |
| TM-01 | Критический | Своевременное обновление QEMU/KVM/libvirt и применение VEX/CVE-патчей; сохранение seccomp/SELinux и whitelist блочных драйверов; минимизация привилегий `virt-launcher`, где возможно; фаззинг эмуляции устройств и sanitizer-сборки; изоляция узлов с ВМ. |
| TM-02 | Высокий | Минимизировать назначения роли `privileged-user`; аудит доступа к console/vnc/portforward; проверка корректности delegating authorization агрегированного API; тесты обхода авторизации по subresources. |
| TM-03 | Высокий | Усилить admission-валидацию provisioning-данных и привязок дисков/образов (VMBDA); ограничить право `create/update` ресурсов ВМ; ревью валидаторов CRD; проверка cross-namespace ссылок. |
| TM-04 | Высокий | Обеспечить HA и доступность `virtualization-controller`; мониторинг ошибок admission; нагрузочное тестирование плоскости управления. Применить **явные `failurePolicy: Fail` и `timeoutSeconds`** ко всем webhook'ам в `templates/virtualization-controller/{validation,mutating}-webhook.yaml` (сейчас `failurePolicy` не задана явно). Сохранить `ValidatingAdmissionPolicy virtualization-restricted-access-policy` с `failurePolicy: Fail` и `validationActions: [Deny]` (сильная компенсирующая мера против записи во внутренние CRD CDI/KubeVirt произвольными СО). |
| TM-05 | Критический | Сократить агрегацию `view_secrets` до минимально необходимого: `templates/rbacv2/use/capabilities/view_secrets.yaml` агрегирует `get/list/watch secrets` в базовую роль `user` модуля (через лейбл `rbac.deckhouse.io/aggregate-to-virtualization-as: user`) — рекомендуется перенести capability в `editor`/`admin` или ограничить `resourceNames` (только cloud-init/sysprep секреты ВМ). Ограничить RBAC на Secret до namespace/resourceNames; включить аудит чтения Secret; ротация TLS-ключей и учётных данных DVCR. |
| TM-06 | Высокий | В `crds/virtualimages.yaml`/`clustervirtualimages.yaml` поля `http.checksum` (md5/sha256) и `caBundle` — **опциональны**, `http.url` допускает плайн `http://`, криптоподпись образа не проверяется. Меры: орг-политикой/admission требовать обязательный `checksum` (sha256) и запретить плайн HTTP/skip-TLS для VI/CVI; ввести allowlist registry/хостов; рассмотреть проверку подписи (cosign) для `ContainerImage`; фаззинг qcow2/vmdk-парсинга (qemu-img/`nbdkit v1.39.5`/`libnbd v1.23.6`); SCA форка CDI `v1.60.3-v12n.19` (SBOM Go-биндингов — `tmp/sbom-output/main/cdiimporter/packages.tsv`; нативные парсеры qemu-img/nbdkit/libnbd зафиксированы по SHA-256 в `cdiimporter/native-artifacts.tsv`, но не привязаны к CVE). |
| TM-07 | Критический | В чарте **отсутствуют NetworkPolicy/CiliumNetworkPolicy** в namespace `d8-virtualization` — обязательно добавить default-deny и явные allow-правила между `virt-handler` ↔ `virt-launcher` ↔ `virt-api` ↔ `cdi-*` ↔ `dvcr` ↔ `kube-api-rewriter`, используя CiliumNetworkPolicy (модуль требует `cni-cilium`). `virt-handler` уже явно `privileged: true` + `runAsUser: 0` + `hostPID` + `hostNetwork` + hostPath к `/var/lib/kubelet`, `/var/run/kubevirt*` (`SecurityPolicyException virt-handler-ds`) — это подтверждает критичность TM-07. Для `virtualization-dra` и `vm-route-forge` — hostNetwork + hostPath к `/lib/modules`, `/sys`, `/var/lib/kubelet/plugins`, route table 1490 (поверхность расширяет TM-07 и TM-08). Меры: строгая NetworkPolicy; изоляция namespace `d8-virtualization` (ограничение прав `nodes/proxy`, `pods/exec` извне); контроль `SYS_MODULE`; SELinux MCS-изоляция virt-launcher; фаззинг/ревью privileged-контейнеров. |
| TM-08 | Высокий | Контроль непересечения `virtualMachineCIDRs` с подсетями pod/service/node; мониторинг изменений route table `1490` и eBPF; ограничение прав на VirtualMachineIPAddress; тесты подмены IP/MAC. |
| TM-09 | Критический | SBOM по 21 релизному образу предоставлен (`tmp/sbom-output/main/`, CycloneDX/SPDX + Trivy + OpenVEX + нативный слой native-artifacts.tsv по SHA-256); **сопоставить нативный слой с ОС-пакетами/CVE** через `trivy fs` по сборочным стадиям (`*-artifact`, особенно `virt-artifact`/`cdi-artifact`/`dvcr-artifact` с CGO-зависимостями QEMU/libvirt/nbdkit/libxml2) и **наполнить OpenVEX-statements** (автогенерация есть, но `statements: []` в текущем прогоне); закреплять commit/tag и проверять подписи/хэши (`werf-giterminism.yaml`); защита `SOURCE_REPO`/`GOPROXY`/registry/CI-секретов; SCA/SAST для локальных патчей. |
| TM-10 | Средний | Ограничить RBAC на метрики; закрыть/исключить pprof и `logLevel: debug` в production; исключить чувствительные данные из логов; включить `virtualization-audit` (EE) и контролировать его целостность; настроить retention. |

<div class="tm-section-gap tm-section-gap-md"></div>

**Компоненты, подлежащие тестированию:**

<div class="tm-table tm-table-testing"></div>

| Компонент | Виды тестирования | Связанные угрозы | Цель тестирования |
| --- | --- | --- | --- |
| virt-launcher / QEMU v9.2.0 / libvirt v10.9.0 | Fuzzing, sanitizer (ASAN/UBSAN), регрессионные тесты | TM-01 | Поиск escape/уязвимостей эмуляции устройств, проверка seccomp/SELinux/whitelist |
| virtualization-api (агрегированный API) | DAST, тесты авторизации | TM-02 | Корректность authn/authz subresources console/vnc/portforward |
| virtualization-controller (admission/conversion) | DAST (admission), регрессионные/модульные тесты | TM-03, TM-04 | Валидация CRD/provisioning, устойчивость при недоступности вебхука |
| dvcr-importer / CDI importer / dvcr-uploader | Fuzzing, DAST | TM-06 | Парсинг qcow2/raw/iso, проверка источников, обработка ошибок |
| DVCR (distribution v2.8.3 + cleaner) | DAST, регрессионные тесты | TM-05, TM-06 | Аутентификация push/pull, изоляция блобов, GC |
| vm-route-forge | Регрессионные тесты, тесты privileged-контейнеров | TM-08 | Корректность route table 1490/eBPF, поведение при сбое |
| virt-handler / virtualization-dra | Тесты privileged-контейнеров, регрессионные тесты | TM-07 | Корректность hostPath/hostPID/`SYS_MODULE`, отсутствие выхода на узел |
| RBAC (user-authz, rbacv2) | Configuration review, тесты авторизации | TM-02, TM-05 | Минимальность прав, обоснованность `view_secrets`/`nodes/proxy` |
| kube-rbac-proxy / метрики/логи | Регрессионные тесты | TM-10 | Ограничение доступа (SubjectAccessReview), отсутствие утечек |
| Build/update flow (`werf.inc.yaml`, patches) | SCA/SAST, supply-chain тесты | TM-09 | Целостность зависимостей, подписи, provenance |

<div class="tm-section-gap tm-section-gap-md"></div>

**План проверки безопасности:**

<div class="tm-table tm-table-checks"></div>

| Направление | Проверки |
| --- | --- |
| DAST/penetration testing | Авторизация subresources console/vnc/portforward; обход admission; доступ к чужим ВМ/дискам; DVCR push/pull auth; доступность вебхуков. |
| Fuzzing | Эмуляция устройств QEMU; парсинг образов qcow2/raw/iso (qemu-img/nbdkit); admission-объекты; gRPC DRA. |
| Code review | Локальные патчи KubeVirt/CDI/QEMU/libvirt/edk2; admission/conversion-валидаторы; обработка DVCR-секретов; vm-route-forge route updates; NodeGroupConfiguration-скрипты; werf-шаблоны и RBAC. |
| Configuration review | Фактические `ModuleConfig`, RoleBinding, NetworkPolicy, экспонирование Service, audit/retention, квоты/лимиты, debug/logLevel, featureGates, `failurePolicy`. |
| Supply-chain review | SBOM/VEX релизных образов (см. `tmp/sbom-output/main/` — 21 образ CycloneDX/SPDX, Trivy-инвентаризация Go-бинарников + нативный слой native-artifacts.tsv по SHA-256, OpenVEX); CVE-привязка нативного слоя (rpm/apk сборочных стадий) и наполнение VEX-statements; форк-коммиты KubeVirt/CDI; теги QEMU/libvirt/edk2/distribution; digest базовых образов; Go-module replacements; allowlist `werf-giterminism.yaml`; подписи/provenance. |

---

> **Трассируемость «угроза → сценарий → решение» (контроль полноты модели по ПРИЛОЖЕНИЮ 2):** покрыты все компоненты разделов 2–3 и все категории STRIDE; каждая угроза TM-01…TM-10 сопоставлена со сценариями AS-01…AS-14 и решением об актуальности. Вспомогательная фактура (подтверждение архитектурных предположений по репозиторию, история CVE по версиям компонентов, привязка к MITRE ATT&CK/CAPEC и план митигации с YAML-правками) вынесена в отдельный документ `virtualization-threat-model-appendices.md`.

