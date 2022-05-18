#!/usr/bin/python

# file in format CONTRIB_EXTENSIONS = {...}
execfile('contrib/contrib_build_config.bzl')

enabled = [
    "envoy.filters.network.kafka_broker"
]

disabled = []
for k, v in CONTRIB_EXTENSIONS.items():
    disabled.append('--{target}:enabled={enabled}'.format(
        target=v.split(":")[0],
        enabled=(k in enabled))
    )

print(' '.join(disabled))
