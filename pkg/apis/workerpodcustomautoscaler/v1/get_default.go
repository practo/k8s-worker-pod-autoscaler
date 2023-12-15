package v1

func (w *WorkerPodCustomAutoScaler) GetMaxDisruption(defaultDisruption string) *string {
	if w.Spec.MaxDisruption == nil {
		return &defaultDisruption
	}
	return w.Spec.MaxDisruption
}
