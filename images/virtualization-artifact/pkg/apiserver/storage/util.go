package storage

// func FetchVirtualMachine(vmLister cache.GenericLister, name, namespace string) (*virtv2.VirtualMachine, *errors.StatusError) {
//	obj, err := vmLister.ByNamespace(namespace).Get(name)
//	if err != nil {
//		if errors.IsNotFound(err) {
//			return nil, errors.NewNotFound(virtv2.Resource("virtualmachine"), name)
//		}
//		return nil, errors.NewInternalError(fmt.Errorf("unable to retrieve vm [%s]: %w", name, err))
//	}
//	if vm, ok := obj.(*virtv2.VirtualMachine); ok {
//		return vm, nil
//	}
//	return nil, errors.NewInternalError(fmt.Errorf("unable to retrieve vm [%s]: %w", name, err))
//}
