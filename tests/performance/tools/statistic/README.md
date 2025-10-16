# Statistic Tool

Утилита для сбора статистики по виртуальным машинам, дискам и операциям в Kubernetes кластере.

## Возможности

- Сбор статистики по VirtualMachine (VM)
- Сбор статистики по VirtualDisk (VD) 
- Сбор статистики по VirtualMachineOperation (VMOP)
- Фильтрация по количеству ресурсов
- Указание папки для сохранения файлов
- Раздельные отчеты для разных типов ресурсов

## Использование

### Основные команды

```bash
# Собрать статистику по всем ресурсам
go run cmd/statistic/main.go -n <namespace>

# Собрать статистику только по VM
go run cmd/statistic/main.go -v -n <namespace>

# Собрать статистику только по VD
go run cmd/statistic/main.go -d -n <namespace>

# Собрать статистику только по VMOP
go run cmd/statistic/main.go -o -n <namespace>
```

### Параметры

- `-n, --namespace` - namespace для поиска ресурсов (по умолчанию: "perf")
- `-v, --virtualmachine` - собрать статистику по VM
- `-d, --virtualdisk` - собрать статистику по VD
- `-o, --vmop` - собрать статистику по VMOP
- `-O, --output-dir` - папка для сохранения CSV файлов (по умолчанию: ".")
- `-c, --vm-count` - ограничить количество VM для обработки (0 = все)
- `-C, --vd-count` - ограничить количество VD для обработки (0 = все)

### Примеры использования

```bash
# Собрать статистику по 10 VM в namespace "perf" и сохранить в папку "reports"
go run cmd/statistic/main.go -v -n perf -O reports -c 10

# Собрать статистику по всем VMOP в namespace "test"
go run cmd/statistic/main.go -o -n test -O /tmp/statistics

# Собрать статистику по 5 VD и 5 VM
go run cmd/statistic/main.go -v -d -n perf -c 5 -C 5 -O ./results
```

## Использование через Taskfile

```bash
# Собрать статистику по VM с ограничением количества
task get-stat:vm NAMESPACE=perf OUTPUT_DIR=./reports VM_COUNT=10

# Собрать статистику по VMOP
task get-stat:vmop NAMESPACE=perf OUTPUT_DIR=./reports

# Собрать статистику по всем ресурсам
task get-stat:all NAMESPACE=perf OUTPUT_DIR=./reports VM_COUNT=5 VD_COUNT=5
```

## Выходные файлы

Утилита создает следующие файлы:

- `all-vm-<namespace>-<timestamp>.csv` - детальная статистика по VM
- `all-vd-<namespace>-<timestamp>.csv` - детальная статистика по VD  
- `all-vmop-<namespace>-<timestamp>.csv` - детальная статистика по VMOP
- `avg-vm-<namespace>-<timestamp>.csv` - средние значения по VM
- `avg-vd-<namespace>-<timestamp>.csv` - средние значения по VD
- `avg-vmop-<namespace>-<timestamp>.csv` - средние значения по VMOP

## Статистика по VM

- **WaitingForDependencies** - время ожидания зависимостей
- **VirtualMachineStarting** - время запуска виртуальной машины
- **GuestOSAgentStarting** - время запуска гостевого агента

## Статистика по VD

- **WaitingForDependencies** - время ожидания зависимостей
- **DVCRProvisioning** - время подготовки DVCR
- **TotalProvisioning** - общее время подготовки

## Статистика по VMOP

- **Phase** - текущая фаза операции
- **Duration** - продолжительность операции
- **StartTime** - время начала операции
- **EndTime** - время окончания операции

## Интеграция с тестами производительности

Утилита интегрирована с тестами производительности и может использоваться для:

1. Сбора статистики по определенному количеству VM (например, по 10% VM)
2. Раздельного сбора статистики по VM и VMOP
3. Сохранения отчетов в структурированные папки

Пример использования в тестах:

```bash
# Собрать статистику только по 10% VM (2 VM из 20)
task get-stat:vm NAMESPACE=perf OUTPUT_DIR=./scenario_1_persistentVolumeClaim/statistics VM_COUNT=2

# Собрать статистику по VMOP после миграции
task get-stat:vmop NAMESPACE=perf OUTPUT_DIR=./scenario_1_persistentVolumeClaim/statistics
```
