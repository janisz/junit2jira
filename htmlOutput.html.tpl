<html>
<head>
<title>Possible Flake Tests</title>
<style>
body { color: #e8e8e8; background-color: #424242; font-family: "Roboto", "Helvetica", "Arial", sans-serif }
a { color: #ff8caa }
a:visited { color: #ff8caa }
</style>
</head>
<body>
<ul>
{{- $url := .JiraUrl -}}
{{- range $issue := .Issues }}
<li><a target=_blank href="{{ $url.Parse ( print "browse/" $issue.Key ) }}">
{{- $issue.Key }}: {{ if $issue.Fields }}{{ $issue.Fields.Summary }}{{ end -}}
</a>
{{- end }}
</ul>
<br />{{- /* Workaround for PROW iframe height calculation */ -}}
<br />
</body>
</html>