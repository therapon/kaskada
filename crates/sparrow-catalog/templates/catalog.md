# Functions

| Function                                    | Summary                        |
|:--------------------------------------------|:-------------------------------|
{% for function in functions -%}
| [{{ function.name }}](#{{ function.name }}) | {{function.short_doc | trim }} |
{% endfor %}

# Function Categories
## Operators
| Function                                    | Summary                        |
|:--------------------------------------------|:-------------------------------|
{% for function in functions | filter(attribute="operator") -%}
| [{{ function.operator }}](#{{ function.name }}) | {{function.short_doc | trim }} |
{% endfor -%}

{% for tag in tags | sort %}
## {{ tag | capitalize }} Functions
{% if tag == 'aggregation' -%}
Aggregation functions provide the mechanism for computing across rows.
The result of an aggregation represents the aggregate result for each
key up to and including the current row. This approximately corresponds to
the result you would get if you ran a SQL aggregation over the values
available at the time of that row.

Aggregations may be configured to operate in a specific window by providing
a [window function](#window-functions) as the optional `window` argument.
If no window is provided, the aggregation is over all rows for the entity,
up to and including the current time. If a window is provided, the result
of an aggregation is the result for that entity in the current window up
to and including the current time. The current window is often not yet
complete.

NOTE: All aggregations in Fenl are implicitly scoped to the entity key.
This would be equivalent to performing a grouped aggregation in SQL.
{%- endif %}

| Function                                    | Summary                        |
|:--------------------------------------------|:-------------------------------|
{% for function in functions -%}
{% if function.tags is containing(tag) -%}
| [{{ function.name }}](#{{ function.name }}) | {{function.short_doc | trim }} |
{% endif -%}
{% endfor -%}
{% endfor %}
# Function Details

{% for function in functions -%}
{% include "partials/function.md" %}
{% endfor -%}