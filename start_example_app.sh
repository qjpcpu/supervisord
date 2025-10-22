#!/bin/bash

mkdir -pv example_app/demo/{bin,conf}
rm -fr example_app/demo/bin/*
rm -fr example_app/demo/conf/*
(cd example_app/demo/bin && ln -sv ../../../bin/supervisord .)
(cd example_app && go build && mv -f example_app demo/bin)
cd example_app/demo && bin/supervisord start -supvr.admin_sock socks/sup.sock  -supvr.exit_when_all_done true DEMO -supvr.stdout log/stdout.log ./bin/example_app  --run_timeout 260  #--swallow_signal TERM
#cd example_app/demo && bin/supervisord start -supvr.admin_sock super.sock -supvr.exit_when_all_done true DEMO ./bin/example_app  --run_timeout 60 #--swallow_signal TERM
