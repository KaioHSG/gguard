package main

import "embed"

//go:embed resources/*
var embedded embed.FS

var _ = &embedded