package models

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

//go:generate bash ../scripts/generate_protos.sh
//go:generate counterfeiter . LocketClient

const PresenceType = "presence"
const LockType = "lock"

var ErrLockCollision = grpc.Errorf(codes.AlreadyExists, "lock-collision")
var ErrInvalidTTL = grpc.Errorf(codes.InvalidArgument, "invalid-ttl")
var ErrInvalidOwner = grpc.Errorf(codes.InvalidArgument, "invalid-owner")
var ErrResourceNotFound = grpc.Errorf(codes.NotFound, "resource-not-found")
var ErrInvalidType = grpc.Errorf(codes.NotFound, "invalid-type")
