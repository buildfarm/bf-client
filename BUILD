load("@gazelle//:def.bzl", "gazelle")
load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

gazelle(name = "gazelle")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/werkt/bf-client",
    visibility = ["//visibility:private"],
    deps = [
        "//client:go_default_library",
        "//view:go_default_library",
        "@com_github_gizak_termui_v3//:go_default_library",
        "@com_github_gizak_termui_v3//widgets:go_default_library",
        "@com_github_nsf_termbox_go//:go_default_library",
    ],
)

go_binary(
    name = "client",
    embed = [":go_default_library"],
    visibility = ["//visibility:public"],
)
