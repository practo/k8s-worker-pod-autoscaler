package v1

func (w *WorkerPodAutoScaler) GetMaxDisruption(defaultDisruption string) *string {
	if w.Spec.MaxDisruption == nil {
		return &defaultDisruption
	}
	return w.Spec.MaxDisruption
}
