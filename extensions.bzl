load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:local.bzl", "new_local_repository")

def _buildfarm_extension_impl(_ctx):
    http_archive(
        name = "buildfarm",
        build_file = "@//:BUILD.buildfarm",
        sha256 = "63b435de100d3a91e9ee7f2b36566019fb5424f19bf5315e722c56e5723279f3",
        strip_prefix = "buildfarm-fd06ca3334ce5287afbd7387d76555aeab2e02b7/src/main/protobuf/build/buildfarm/v1test/",
        urls = [
            "https://github.com/buildfarm/buildfarm/archive/fd06ca3334ce5287afbd7387d76555aeab2e02b7.zip",
        ],
    )

build_deps = module_extension(
    implementation = _buildfarm_extension_impl,
)
