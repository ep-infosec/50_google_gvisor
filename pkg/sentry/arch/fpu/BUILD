load("//tools:defs.bzl", "go_library")

package(licenses = ["notice"])

go_library(
    name = "fpu",
    srcs = [
        "fpu.go",
        "fpu_amd64.go",
        "fpu_amd64.s",
        "fpu_arm64.go",
        "fpu_unsafe.go",
    ],
    visibility = ["//:sandbox"],
    deps = [
        "//pkg/cpuid",
        "//pkg/errors/linuxerr",
        "//pkg/hostarch",
        "//pkg/sync",
        "@org_golang_x_sys//unix:go_default_library",
    ],
)
