load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

def _buildfarm_extension_impl(_ctx):
    http_archive(
        name = "buildfarm",
        build_file = "@//:BUILD.buildfarm",
        sha256 = "575c657a565c4e986ba10b55c6e652c6ec7a68c2222b4951a9362dc0e53e3d2d",
        strip_prefix = "buildfarm-2.16.0/src/main/protobuf/build/buildfarm/v1test/",
        urls = [
            "https://github.com/buildfarm/buildfarm/archive/refs/tags/2.16.0.zip",
        ],
    )

build_deps = module_extension(
    implementation = _buildfarm_extension_impl,
)
