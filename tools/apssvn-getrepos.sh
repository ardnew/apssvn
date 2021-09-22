#!/bin/bash

curl_bin='curl'
xmllint_bin='xmllint.exe'
tr_bin='tr'
json_pp_bin='json_pp'
perl_bin='perl'

chkdep() {
	[[ ${#} -lt 2 ]] && return -1
	local dep=${1} ret=${2}
	if ! type -t "${dep}" &> /dev/null; then
		printf -- 'error: required dependency not found: %s\n' "${dep}"
		exit ${ret}
	fi
}

chkdep "${curl_bin}"    100
chkdep "${xmllint_bin}" 101
chkdep "${tr_bin}"      102
chkdep "${json_pp_bin}" 103
chkdep "${perl_bin}"    104

# TODO: You need to regenerate the curl command every time this
#       script is run, because it re-uses the session/cookies 
#       created by your Web browser to login and authenticate.
#       Obviously, this isn't ideal, but I'm not sure how to do
#       it any other way while ensuring the entire list embedded
#       in the HTML+JS source gets scraped. (FIXME)
#
# To generate the required curl command, visit the following URL 
# using Google Chrome: 
#
#   http://rstok3-dev02:3343/csvn/repo/list
#
# Once the login page appears, open the Chrome menu, navigate to:
#
#   More Tools > Developer Tools, and then open the Network tab.
#
# Then inside the browser pane, login normally with your DevNet
# username and password. The network activity will appear in the
# Developer Tools. You want to find the request with name "list",
# which is the name of the resource we originally requested via
# URL (the Type will be "document", and Status should be 200).
#
# Right-click on this request, expand the Copy sub-menu, and then
# select "Copy as cURL (bash)". Paste the results. Be sure to add
# the output pipe to xmllint (and options --silent, --show-error)
# following the cURL command, which is indented 2 extra levels! 
#
# Leave all other commands in the pipeline as-is!


"${curl_bin}" 'http://rstok3-dev02:3343/csvn/repo/list' \
			-H 'Connection: keep-alive' \
			-H 'Cache-Control: max-age=0' \
			-H 'Upgrade-Insecure-Requests: 1' \
			-H 'User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/93.0.4577.63 Safari/537.36' \
			-H 'Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9' \
			-H 'Referer: http://rstok3-dev02:3343/csvn/login/auth' \
			-H 'Accept-Language: en-US,en;q=0.9' \
			-H 'Cookie: SESSID=wz67e7i57vecwoaws8bufbh5; remember=g81222:408:f77850c77e23282e8efeab94f2557ffd; crucibleprefs1=D%3D1631722214409%3Bssyn%3Dpl%3Bhdf%3DY%3Bdm%3Dy; FESESSIONID=node0143u6wjknwg27we87toqk4b2043.node0; atl.xsrf.token.slash=5c448e47210e8760cfd5efc411fdf850f34b2f9c; jira.editor.user.mode=source; JSESSIONID=A72880191A2038D6A9A09583C4077B9A; atlassian.xsrf.token=BPFU-2141-1CA8-WD4D_3956985eaafdf28dc92e932be99fad4d9996ae36_lin' \
			--compressed \
			--insecure \
	--silent \
	--show-error \
	| "${xmllint_bin}"    \
		--html        \
		--nonet       \
		--format      \
		--noblanks    \
		--nowarning   \
		--xpath 'substring-after(//script[contains(text(),"/* Data set */")]/text(), "=")' \
		-             \
		2>/dev/null   \
	| "${tr_bin}"         \
		\' \"         \
	| "${tr_bin}"         \
		-d \;         \
	| "${json_pp_bin}"    \
		-t dumper     \
	| "${perl_bin}"       \
		-e '$/=$\;$_=<>;$r=eval;print"$_\n"for map{${$_}[1]}@{$r}'


