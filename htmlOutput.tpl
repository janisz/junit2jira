<html>
    <head>
        <title><h4>Possible Flake Tests</h4></title>
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
        <li><a target=_blank href="{{ $url }}browse/{{ $issue.Key }}">
        {{- $issue.Key }}: {{ if $issue.Fields }}{{ $issue.Fields.Summary }}{{ end -}}
        </a>
    {{- end }}
    </ul>
  </body>
</html>