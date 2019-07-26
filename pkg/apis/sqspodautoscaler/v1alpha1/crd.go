package v1alpha1

// import (
// 	// "reflect"
//
// 	// "github.com/golang/glog"
// 	// "github.com/practo/k8s-sqs-pod-autoscaler-controllers/pkg/apis/sqspodautoscaler"
// 	//
// 	// apiextensionv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
// 	// apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
// 	// apierrors "k8s.io/apimachinery/pkg/api/errors"
// 	// meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// )
//
// const (
// 	CRDSingular  string = "sqspodautoscaler"
// 	CRDPlural    string = CRDSingular + "s"
// 	CRDShortName string = "spa"
// 	Version      string = "v1alpha1"
// 	FullCRDName  string = CRDPlural + "." + sqspodautoscaler.GroupName
// )
//
// // func CreateCRD(clientset apiextension.Interface) error {
// // 	crd := &apiextensionv1beta1.CustomResourceDefinition{
// // 		ObjectMeta: meta_v1.ObjectMeta{Name: FullCRDName},
// // 		Spec: apiextensionv1beta1.CustomResourceDefinitionSpec{
// // 			Group:   sqspodautoscaler.GroupName,
// // 			Version: Version,
// // 			Scope:   apiextensionv1beta1.NamespaceScoped,
// // 			Names: apiextensionv1beta1.CustomResourceDefinitionNames{
// // 				Singular:   CRDSingular,
// // 				Plural:     CRDPlural,
// // 				ShortNames: []string{CRDShortName},
// // 				Kind:       reflect.TypeOf(SqsPodAutoScaler{}).Name(),
// // 			},
// // 		},
// // 	}
// //
// // 	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
// // 	if err != nil && apierrors.IsAlreadyExists(err) {
// // 		glog.Infof("CRD %s already exists", CRDPlural)
// // 		return nil
// // 	}
// // 	glog.Infof("CRD %s created", CRDPlural)
// // 	return err
// // }
