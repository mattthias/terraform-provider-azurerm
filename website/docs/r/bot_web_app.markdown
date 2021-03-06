---
layout: "azurerm"
page_title: "Azure Resource Manager: azurerm_bot_web_app"
sidebar_current: "docs-azurerm-resource-bot-web-app"
description: |-
  Manages a Web App Bot.
---

# azurerm_bot_web_app

Manages a Web App Bot.

## Example Usage

```hcl
data "azurerm_client_config" "current" {}

resource "azurerm_resource_group" "example" {
  name     = "example"
  location = "northeurope"
}

resource "azurerm_bot_web_app" "example" {
  name                = "example"
  location            = "global"
  resource_group_name = "${azurerm_resource_group.example.name}"
  sku                 = "F0"
  microsoft_app_id    = "${data.azurerm_client_config.current.service_principal_application_id}"
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required) Specifies the name of the Web App Bot. Changing this forces a new resource to be created. Must be globally unique.

* `resource_group_name` - (Required) The name of the resource group in which to create the Web App Bot. Changing this forces a new resource to be created.

* `location` - (Required) The supported Azure location where the resource exists. Changing this forces a new resource to be created.

* `sku` - (Required) The SKU of the Web App Bot. Valid values include `F0` or `S1`. Changing this forces a new resource to be created.

* `microsoft_app_id` - (Required) The Microsoft Application ID for the Web App Bot. Changing this forces a new resource to be created.

* `display_name` - (Optional) The name of the Web App Bot will be displayed as. This defaults to `name` if not specified. 

* `endpoint` - (Optional) The Web App Bot endpoint.

* `developer_app_insights_key` - (Optional) The Application Insights Key to associate with the Web App Bot.

* `developer_app_insights_api_key` - (Optional) The Application Insights API Key to associate with the Web App Bot.

* `developer_app_insights_application_id` - (Optional) The Application Insights Application ID to associate with the Web App Bot.

* `luis_app_ids` - (Optional) A list of LUIS App IDs to associate with the Web App Bot.

* `luis_key` - (Optional) The LUIS key to associate with the Web App Bot.

* `tags` - (Optional) A mapping of tags to assign to the resource.


## Attributes Reference

The following attributes are exported:

* `id` - The Web App Bot ID.

## Import

Web App Bots can be imported using the `resource id`, e.g.

```shell
terraform import azurerm_bot_web_app.example /subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/example/providers/Microsoft.BotService/botServices/example
```
