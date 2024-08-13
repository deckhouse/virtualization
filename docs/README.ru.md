---
title: "Deckhouse Virtualization Platform"
menuTitle: "Deckhouse Virtualization Platform"
moduleStatus: preview
weight: 10
---


Deckhouse Virtualization Platform позволяет декларативно создавать, запускать и управлять виртуальными машинами и их ресурсами.

## Сценарии использования

- Запуск виртуальных машин с x86_64 совместимой ОС.
- Запуска виртуальных машин и контейнеризованных приложений в одном окружении.

  ![](./images/cases-vms.ru.png)

  ![](./images/cases-pods-and-vms.ru.png)

{{< alert level="warning" >}}
Если вы планируете использовать Deckhouse Virtualization Platform в production-среде, рекомендуется разворачивать его на физических серверах. Развертывание модуля на виртуальных машинах также возможно, но в этом случае необходимо включить nested-виртуализацию.
{{< /alert >}}

Для работы виртуализации требуется кластер Deckhouse Kubernetes Platform. Пользователям редакции Enterprise Edition доступна возможность управления ресурсами через графический интерфейс (UI).

Для подключения к виртуальным машинам с использованием последовательного порта, VNC или по протоколу ssh используется утилита командной строки [d8](https://deckhouse.ru/documentation/v1/deckhouse-cli/).
