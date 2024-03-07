package e2e

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/deckhouse/virtualization/tests/e2e/helper"
	. "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type virtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func getVMFromManifest(manifest string) (*virtualMachine, error) {
	unstructs, err := helper.ParseYaml(manifest)
	if err != nil {
		return nil, err
	}
	var unstruct *unstructured.Unstructured
	for _, u := range unstructs {
		if helper.GetFullApiResourceName(u) == kc.ResourceVM {
			unstruct = u
			break
		}
	}
	var vm virtualMachine

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.Object, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

var _ = Describe("Label and Annotation", Ordered, ContinueOnFailure, func() {
	imageManifest := vmPath("image.yaml")
	vmManifest := vmPath("vm_02_connectivity_service.yaml")

	BeforeAll(func() {
		By("Apply image for vms")
		ApplyFromFile(imageManifest)
		WaitFromFile(imageManifest, PhaseReady, LongWaitDuration)
	})
	AfterAll(func() {
		By("Delete all manifests")
		files := make([]string, 0)
		err := filepath.Walk(conf.VM.TestDataDir, func(path string, info fs.FileInfo, err error) error {
			if err == nil && strings.HasSuffix(info.Name(), "yaml") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil || len(files) == 0 {
			kubectl.Delete(imageManifest, kc.DeleteOptions{})
			kubectl.Delete(conf.VM.TestDataDir, kc.DeleteOptions{})
		} else {
			for _, f := range files {
				kubectl.Delete(f, kc.DeleteOptions{})
			}
		}
	})

	// test here
	// 8.5 Подключить к ВМ напрямую с ноды
	//  Подключится по ssh в вм с ноды
	// 8.6 Проверки связности с ВМ как сервисом
	//  Связь ВМ с внешним миром
	//     Создать VM
	//     Подключить к VM, далее выполнить curl https://flant.com
	//  Связь внешнего мира с ВМ
	//     Создать VM
	//     Установить nginx и настроить его на обслуживание его на порт 80
	//     Создать сервис vm-svc1 (lb или nodePort) и привязать его к лейблу service: v1
	//     Создать сервис vm-svc2 (lb или nodePort) и привязать его к лейблу service: v2
	//     Получить доступ к vm-svc1
	//     Изменить лейбл вм c sevice: v1 -> v2
	//     Получить доступ к vm-svc2
	// 8.8 DVCR
	//  Размеры DVCR
	//     Увеличить размер DVCR
	// 9.1 Cloud
	//	Создать несколько VM (>20)
	//	Удалить созданные на предыщущем шаге ресурсы

})
