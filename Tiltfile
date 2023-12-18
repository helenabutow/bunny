# -*- mode: Python -*-

# For more on Extensions, see: https://docs.tilt.dev/extensions.html
load('ext://restart_process', 'docker_build_with_restart')

secret_settings(disable_scrub=True)

# the gcflags come from the delve debugger requesting it at https://github.com/go-delve/delve/blob/master/Documentation/usage/dlv_exec.md
compile_cmd = 'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -gcflags="all=-N -l" -o bunny ./'

local_resource(
  'bunny-compile',
  compile_cmd,
  deps=['./main.go', './config/config.go', './ingress/ingress.go']
)

docker_build_with_restart(
  'bunny-image',
  '.',
  entrypoint=['/root/go/bin/dlv', '--listen=0.0.0.0:50666', '--api-version=2', '--headless=true', '--only-same-user=false', '--accept-multiclient', '--check-go-version=false', 'exec', '/bunny'],
  # entrypoint=['/root/go/bin/dlv', 'dap', '--listen', ':50666', '/bunny'],
  dockerfile='deploy/containers/bunny/Dockerfile',
  only=[
    './bunny',
  ],
  live_update=[
    sync('./bunny', '/bunny'),
    sync('./', '/src/'),
  ],
)

k8s_yaml('deploy/kubernetes/bunny-deployment.yaml')

# even though Tilt detects the file change and applies the change against Kubernetes immediately,
# expect fsnotify in the container to take about a minute before it picks up the change
# I suspect that this is because of: https://github.com/kubernetes/kubernetes/issues/113746
# with a potential fix in: https://github.com/kubernetes/kubernetes/pull/117431
k8s_yaml('deploy/kubernetes/bunny-secret.yaml')

k8s_resource(
    'bunny',
    port_forwards=[
      "1312:1312", # the regular port for bunny
      "50666:50666", # the port for connecting the debugger
    ],
    resource_deps=['bunny-compile']
)