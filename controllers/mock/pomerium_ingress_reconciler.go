// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/pomerium/ingress-controller/pomerium (interfaces: IngressReconciler)
//
// Generated by this command:
//
//	mockgen -package mock_test -destination pomerium_ingress_reconciler.go github.com/pomerium/ingress-controller/pomerium IngressReconciler
//

// Package mock_test is a generated GoMock package.
package mock_test

import (
	context "context"
	reflect "reflect"

	model "github.com/pomerium/ingress-controller/model"
	gomock "go.uber.org/mock/gomock"
	types "k8s.io/apimachinery/pkg/types"
)

// MockIngressReconciler is a mock of IngressReconciler interface.
type MockIngressReconciler struct {
	ctrl     *gomock.Controller
	recorder *MockIngressReconcilerMockRecorder
}

// MockIngressReconcilerMockRecorder is the mock recorder for MockIngressReconciler.
type MockIngressReconcilerMockRecorder struct {
	mock *MockIngressReconciler
}

// NewMockIngressReconciler creates a new mock instance.
func NewMockIngressReconciler(ctrl *gomock.Controller) *MockIngressReconciler {
	mock := &MockIngressReconciler{ctrl: ctrl}
	mock.recorder = &MockIngressReconcilerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockIngressReconciler) EXPECT() *MockIngressReconcilerMockRecorder {
	return m.recorder
}

// Delete mocks base method.
func (m *MockIngressReconciler) Delete(arg0 context.Context, arg1 types.NamespacedName) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", arg0, arg1)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Delete indicates an expected call of Delete.
func (mr *MockIngressReconcilerMockRecorder) Delete(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockIngressReconciler)(nil).Delete), arg0, arg1)
}

// Set mocks base method.
func (m *MockIngressReconciler) Set(arg0 context.Context, arg1 []*model.IngressConfig) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Set", arg0, arg1)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Set indicates an expected call of Set.
func (mr *MockIngressReconcilerMockRecorder) Set(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Set", reflect.TypeOf((*MockIngressReconciler)(nil).Set), arg0, arg1)
}

// Upsert mocks base method.
func (m *MockIngressReconciler) Upsert(arg0 context.Context, arg1 *model.IngressConfig) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Upsert", arg0, arg1)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Upsert indicates an expected call of Upsert.
func (mr *MockIngressReconcilerMockRecorder) Upsert(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Upsert", reflect.TypeOf((*MockIngressReconciler)(nil).Upsert), arg0, arg1)
}
