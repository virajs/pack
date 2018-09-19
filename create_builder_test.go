package pack_test

import (
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/mocks"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"math/rand"
	"testing"
	"time"
)

func TestFS(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "fs", testBuilderFactory, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package mocks -destination mocks/fs.go github.com/buildpack/pack FS

func testBuilderFactory(t *testing.T, when spec.G, it spec.S) {
	//var factory *pack.BuilderFactory
	//
	//it.Before(func(){
	//	mockController := gomock.NewController(t)
	//	mockFS := mocks.NewMockFS(mockController)
	//	factory = &pack.BuilderFactory{
	//	}
	//})
	//
	//it("add a layer for each buildpack", func(){
    //    err := factory.Create(pack{})
	//	if err != nil {
	//		t.Fatalf("failed to create builder: %s", err)
	//	}
	//})
}
