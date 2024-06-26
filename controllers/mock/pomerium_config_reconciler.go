// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/pomerium/ingress-controller/pomerium (interfaces: ConfigReconciler)
//
// Generated by this command:
//
//	mockgen -package mock_test -destination pomerium_config_reconciler.go github.com/pomerium/ingress-controller/pomerium ConfigReconciler
//

// Package mock_test is a generated GoMock package.
package mock_test

import (
	context "context"
	reflect "reflect"

	model "github.com/pomerium/ingress-controller/model"
	gomock "go.uber.org/mock/gomock"
)

// MockConfigReconciler is a mock of ConfigReconciler interface.
type MockConfigReconciler struct {
	ctrl     *gomock.Controller
	recorder *MockConfigReconcilerMockRecorder
}

// MockConfigReconcilerMockRecorder is the mock recorder for MockConfigReconciler.
type MockConfigReconcilerMockRecorder struct {
	mock *MockConfigReconciler
}

// NewMockConfigReconciler creates a new mock instance.
func NewMockConfigReconciler(ctrl *gomock.Controller) *MockConfigReconciler {
	mock := &MockConfigReconciler{ctrl: ctrl}
	mock.recorder = &MockConfigReconcilerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConfigReconciler) EXPECT() *MockConfigReconcilerMockRecorder {
	return m.recorder
}

// SetConfig mocks base method.
func (m *MockConfigReconciler) SetConfig(arg0 context.Context, arg1 *model.Config) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetConfig", arg0, arg1)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SetConfig indicates an expected call of SetConfig.
func (mr *MockConfigReconcilerMockRecorder) SetConfig(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetConfig", reflect.TypeOf((*MockConfigReconciler)(nil).SetConfig), arg0, arg1)
}
