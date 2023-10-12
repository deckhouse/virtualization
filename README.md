# Deckhouse virtualization module

# Порядок устанвки модуля в кластер

Данный модуль и внутренний модуль `virtualization` в составе платформы deckhouse имеют одниаковое название, поэтому сборку с ветки `main`, либо любую тегированную использовать нельзя, т.к. будут конфликты. Следует использовать сборку, в которой внутренний модуль `virtualization` удален: https://github.com/deckhouse/deckhouse/pull/5554

`dev-registry.deckhouse.io` переодически очищается от старых артефактов, поэтому если вдруг в кластере возникают ошибки вида `ErrPullImage`, следует перезапустить конвейер сборки для `PR5554`

Переключение на требуемую сборку:

```bash
kubectl -n d8-system set image deploy/deckhouse deckhouse="dev-registry.deckhouse.io/sys/deckhouse-oss:pr5554"
```

Сборка dev-версии модуля осуществляется автоматически сразу после создания MR. В рамках сборки пересобираются все образы модуля (не только модуль контроллера) примерное время сборки всех образов ~30 мин.

Для каждого MR создается свой "канал обновлений", который нужно использовать для тестов.

Пример типовой конфигурации для установки модуля в кластер:

```yaml
---
apiVersion: deckhouse.io/v1alpha1
kind: ModuleSource
metadata:
  name: dev-registry
spec:
  registry:
    dockerCfg: <тут докер конфиг в base64 из файла лицензии>
    repo: dev-registry.deckhouse.io/modules
  # порядковый номер МР
  releaseChannel: mr-69
---
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: virtualization
spec:
  enabled: true
  settings:
    vmCIDRs:
      - 10.66.10.0/24
      - 10.66.20.0/24
  version: 1
```

Прсмотреть конкретно ваш релизный канал можно в финальной джобе `show_module_manifest` конвейера сборки.


Также стоит отметить, что каждый коммит должен быть подписан:

```
git commit -m "fix: some mega fix" --signoff
```

Проверку подписи осуществляет джоба `dco` в рамках конвейера сборки.

Выкат сборки для релизных каналов alpha, beta, etc должен осуществляться только для тегов, но на данный момент этот флоу не тестировался :)
