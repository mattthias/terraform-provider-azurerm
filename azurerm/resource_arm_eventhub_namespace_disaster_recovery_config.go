package azurerm

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/eventhub/mgmt/2017-04-01/eventhub"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/features"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func resourceArmEventHubNamespaceDisasterRecoveryConfig() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmEventHubNamespaceDisasterRecoveryConfigCreateUpdate,
		Read:   resourceArmEventHubNamespaceDisasterRecoveryConfigRead,
		Update: resourceArmEventHubNamespaceDisasterRecoveryConfigCreateUpdate,
		Delete: resourceArmEventHubNamespaceDisasterRecoveryConfigDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: azure.ValidateEventHubAuthorizationRuleName(),
			},

			"namespace_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: azure.ValidateEventHubNamespaceName(),
			},

			"resource_group_name": azure.SchemaResourceGroupName(),

			"partner_namespace_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: azure.ValidateResourceID,
			},

			"alternate_name": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: azure.ValidateEventHubNamespaceName(),
			},

			"wait_for_replication": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
	}
}

func resourceArmEventHubNamespaceDisasterRecoveryConfigCreateUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).eventhub.DisasterRecoveryConfigsClient
	ctx := meta.(*ArmClient).StopContext

	log.Printf("[INFO] preparing arguments for AzureRM EventHub Namespace Disaster Recovery Configs creation.")

	name := d.Get("name").(string)
	namespaceName := d.Get("namespace_name").(string)
	resourceGroup := d.Get("resource_group_name").(string)

	if features.ShouldResourcesBeImported() && d.IsNewResource() {
		existing, err := client.Get(ctx, resourceGroup, namespaceName, name)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("Error checking for presence of existing EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q): %s", name, namespaceName, resourceGroup, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_eventhub_namespace_disaster_recovery_config", *existing.ID)
		}
	}

	parameters := eventhub.ArmDisasterRecovery{
		ArmDisasterRecoveryProperties: &eventhub.ArmDisasterRecoveryProperties{
			PartnerNamespace: utils.String(d.Get("partner_namespace_id").(string)),
		},
	}

	if v, ok := d.GetOk("alternate_name"); ok {
		parameters.ArmDisasterRecoveryProperties.AlternateName = utils.String(v.(string))
	}

	if _, err := client.CreateOrUpdate(ctx, resourceGroup, namespaceName, name, parameters); err != nil {
		return fmt.Errorf("Error creating/updating EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q): %s", name, namespaceName, resourceGroup, err)
	}

	if v, ok := d.GetOkExists("wait_for_replication"); ok && v.(bool) {
		if err := resourceArmEventHubNamespaceDisasterRecoveryConfigWaitForState(ctx, client, resourceGroup, namespaceName, name); err != nil {
			return fmt.Errorf("Error waiting for replication to compelte for EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q): %s", name, namespaceName, resourceGroup, err)
		}
	}
	read, err := client.Get(ctx, resourceGroup, namespaceName, name)
	if err != nil {
		return fmt.Errorf("Error reading EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q): %v", name, namespaceName, resourceGroup, err)
	}

	if read.ID == nil {
		return fmt.Errorf("Got nil ID for EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q)", name, namespaceName, resourceGroup)
	}

	d.SetId(*read.ID)

	return resourceArmEventHubNamespaceDisasterRecoveryConfigRead(d, meta)
}

func resourceArmEventHubNamespaceDisasterRecoveryConfigRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).eventhub.DisasterRecoveryConfigsClient
	ctx := meta.(*ArmClient).StopContext

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	name := id.Path["disasterRecoveryConfigs"]
	resourceGroup := id.ResourceGroup
	namespaceName := id.Path["namespaces"]

	resp, err := client.Get(ctx, resourceGroup, namespaceName, name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error making Read request on Azure EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q): %s", name, namespaceName, resourceGroup, err)
	}

	d.Set("name", name)
	d.Set("namespace_name", namespaceName)
	d.Set("resource_group_name", resourceGroup)

	if properties := resp.ArmDisasterRecoveryProperties; properties != nil {
		d.Set("partner_namespace_id", properties.PartnerNamespace)
		d.Set("alternate_name", properties.AlternateName)
	}

	return nil
}

func resourceArmEventHubNamespaceDisasterRecoveryConfigDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).eventhub.DisasterRecoveryConfigsClient
	ctx := meta.(*ArmClient).StopContext

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	name := id.Path["disasterRecoveryConfigs"]
	resourceGroup := id.ResourceGroup
	namespaceName := id.Path["namespaces"]

	breakPair, err := client.BreakPairing(ctx, resourceGroup, namespaceName, name)
	if breakPair.StatusCode != http.StatusOK {
		return fmt.Errorf("Error issuing break pairing request for EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q): %s", name, namespaceName, resourceGroup, err)
	}

	if err := resourceArmEventHubNamespaceDisasterRecoveryConfigWaitForState(ctx, client, resourceGroup, namespaceName, name); err != nil {
		return fmt.Errorf("Error waiting for break pairing request to compelte for EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q): %s", name, namespaceName, resourceGroup, err)
	}

	delete, err := client.Delete(ctx, resourceGroup, namespaceName, name)
	if delete.StatusCode != http.StatusOK {
		return fmt.Errorf("Error issuing delete request for EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q): %s", name, namespaceName, resourceGroup, err)
	}

	return nil
}

func resourceArmEventHubNamespaceDisasterRecoveryConfigWaitForState(ctx context.Context, client *eventhub.DisasterRecoveryConfigsClient, resourceGroup, namespaceName, name string) error {
	stateConf := &resource.StateChangeConf{
		Pending:    []string{string(eventhub.Accepted)},
		Target:     []string{string(eventhub.Succeeded)},
		Timeout:    30 * time.Minute,
		MinTimeout: 30 * time.Second,
		Refresh: func() (interface{}, string, error) {

			read, err := client.Get(ctx, resourceGroup, namespaceName, name)
			if err != nil {
				return nil, "error", fmt.Errorf("Wait read EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q): %v", name, namespaceName, resourceGroup, err)
			}

			if props := read.ArmDisasterRecoveryProperties; props != nil {
				if props.ProvisioningState == eventhub.Failed {
					return read, "failed", fmt.Errorf("Replication for EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q) failed!", name, namespaceName, resourceGroup)
				}
				return read, string(props.ProvisioningState), nil
			}

			return read, "nil", fmt.Errorf("Waiting for replication error EventHub Namespace Disaster Recovery Configs %q (Namespace %q / Resource Group %q): provisioning state is nil", name, namespaceName, resourceGroup)
		},
	}

	_, err := stateConf.WaitForState()
	return err
}
