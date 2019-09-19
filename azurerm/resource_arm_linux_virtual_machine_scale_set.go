package azurerm

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/validate"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/features"
	computeSvc "github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/services/compute"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tags"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tf/base64"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func resourceArmLinuxVirtualMachineScaleSet() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmLinuxVirtualMachineScaleSetCreate,
		Read:   resourceArmLinuxVirtualMachineScaleSetRead,
		Update: resourceArmLinuxVirtualMachineScaleSetUpdate,
		Delete: resourceArmLinuxVirtualMachineScaleSetDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		// TODO: exposing requireGuestProvisionSignal in the swagger

		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: computeSvc.ValidateLinuxName,
			},

			"resource_group_name": azure.SchemaResourceGroupName(),

			"location": azure.SchemaLocation(),

			// Required
			"admin_username": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validate.NoEmptyStrings,
			},

			"network_interface": computeSvc.VirtualMachineScaleSetNetworkInterfaceSchema(),

			"os_disk": computeSvc.VirtualMachineScaleSetOSDiskSchema(),

			"instances": {
				Type:         schema.TypeInt,
				Required:     true,
				ValidateFunc: validation.IntAtLeast(0),
			},

			"sku": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validate.NoEmptyStrings,
			},

			// Optional
			"additional_capabilities": computeSvc.VirtualMachineScaleSetAdditionalCapabilitiesSchema(),

			"admin_password": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
			},

			"admin_ssh_key": computeSvc.SSHKeysSchema(),

			"computer_name_prefix": {
				Type:     schema.TypeString,
				Optional: true,

				// Computed since we reuse the VM name if one's not specified
				Computed: true,
				ForceNew: true,
				// note: whilst the portal says 1-15 characters it seems to mirror the rules for the vm name
				// (e.g. 1-15 for Windows, 1-63 for Linux)
				ValidateFunc: computeSvc.ValidateLinuxName,
			},

			"custom_data": base64.OptionalSchema(),

			"disable_password_authentication": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true, // TODO: check this default with Azure / raise an error if a passwords specified and no ssh keys?
			},

			"do_not_run_extensions_on_overprovisioned_machines": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"eviction_policy": {
				// only applicable when `priority` is set to `Low`
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(compute.Deallocate),
					string(compute.Delete),
				}, false),
			},

			"overprovision": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"priority": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  string(compute.Regular),
				ValidateFunc: validation.StringInSlice([]string{
					string(compute.Low),
					string(compute.Regular),
				}, false),
			},

			"provision_vm_agent": {
				Type:     schema.TypeBool,
				Optional: true,
				// TODO: check this default
				Default:  true,
				ForceNew: true,
			},

			"proximity_placement_group_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: azure.ValidateResourceID,
			},

			"single_placement_group": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"source_image_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: azure.ValidateResourceID,
			},

			"source_image_reference": computeSvc.VirtualMachineScaleSetSourceImageReferenceSchema(),

			"tags": tags.Schema(),

			"upgrade_mode": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  string(compute.Manual),
				ValidateFunc: validation.StringInSlice([]string{
					string(compute.Automatic),
					string(compute.Manual),
					string(compute.Rolling),
				}, false),
			},

			// TODO: sort these
			"automatic_os_upgrade_policy": computeSvc.VirtualMachineScaleSetAutomatedOSUpgradePolicySchema(),

			"rolling_upgrade_policy": computeSvc.VirtualMachineScaleSetRollingUpgradePolicySchema(),

			"zone_balance": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"zones": azure.SchemaZones(),

			// Computed
			"unique_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceArmLinuxVirtualMachineScaleSetCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).compute.VMScaleSetClient
	ctx := meta.(*ArmClient).StopContext

	resourceGroup := d.Get("resource_group_name").(string)
	name := d.Get("name").(string)

	if features.ShouldResourcesBeImported() {
		resp, err := client.Get(ctx, resourceGroup, name)
		if err != nil {
			if !utils.ResponseWasNotFound(resp.Response) {
				return fmt.Errorf("Error checking for existing Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
			}
		}

		if !utils.ResponseWasNotFound(resp.Response) {
			return tf.ImportAsExistsError("azurerm_linux_virtual_machine_scale_set", *resp.ID)
		}
	}

	location := azure.NormalizeLocation(d.Get("location").(string))
	t := d.Get("tags").(map[string]interface{})

	additionalCapabilitiesRaw := d.Get("additional_capabilities").([]interface{})
	additionalCapabilities := computeSvc.ExpandVirtualMachineScaleSetAdditionalCapabilities(additionalCapabilitiesRaw)

	networkInterfacesRaw := d.Get("network_interface").([]interface{})
	networkInterfaces := computeSvc.ExpandVirtualMachineScaleSetNetworkInterface(networkInterfacesRaw)

	osDiskRaw := d.Get("os_disk").([]interface{})
	osDisk := computeSvc.ExpandVirtualMachineScaleSetOSDisk(osDiskRaw, compute.Linux)

	sourceImageReferenceRaw := d.Get("source_image_reference").([]interface{})
	sourceImageReference := computeSvc.ExpandVirtualMachineScaleSetSourceImageReference(sourceImageReferenceRaw)
	if sourceImageReference == nil {
		sourceImageId := d.Get("source_image_id").(string)
		if sourceImageId == "" {
			return fmt.Errorf("Either a `source_image_id` or a `source_image_reference` block must be specified!")
		}

		sourceImageReference = &compute.ImageReference{
			ID: utils.String(sourceImageId),
		}
	}

	sshKeysRaw := d.Get("admin_ssh_key").(*schema.Set).List()
	sshKeys := computeSvc.ExpandSSHKeys(sshKeysRaw)

	upgradeMode := compute.UpgradeMode(d.Get("upgrade_mode").(string))
	automaticOSUpgradePolicyRaw := d.Get("automatic_os_upgrade_policy").([]interface{})
	automaticOSUpgradePolicy := computeSvc.ExpandVirtualMachineScaleSetAutomaticUpgradePolicy(automaticOSUpgradePolicyRaw)
	if len(automaticOSUpgradePolicyRaw) > 0 && upgradeMode != compute.Automatic {
		return fmt.Errorf("An `automatic_os_upgrade_policy` block cannot be specified when `upgrade_mode` is not set to `Automatic`")
	}
	if upgradeMode == compute.Automatic && len(automaticOSUpgradePolicyRaw) == 0 {
		return fmt.Errorf("An `automatic_os_upgrade_policy` block must be specified when `upgrade_mode` is set to `Automatic`")
	}

	rollingUpgradePolicyRaw := d.Get("rolling_upgrade_policy").([]interface{})
	rollingUpgradePolicy := computeSvc.ExpandVirtualMachineScaleSetRollingUpgradePolicy(rollingUpgradePolicyRaw)
	if len(rollingUpgradePolicyRaw) > 0 && upgradeMode != compute.Rolling {
		return fmt.Errorf("A `rolling_upgrade_policy` block cannot be specified when `upgrade_mode` is not set to `Rolling`")
	}
	if upgradeMode == compute.Rolling && len(rollingUpgradePolicyRaw) == 0 {
		return fmt.Errorf("A `rolling_upgrade_policy` block must be specified when `upgrade_mode` is set to `Rolling`")
	}

	zonesRaw := d.Get("zones").([]interface{})
	zones := azure.ExpandZones(zonesRaw)

	var computerNamePrefix string
	if v, ok := d.GetOk("computer_name_prefix"); ok && len(v.(string)) > 0 {
		computerNamePrefix = v.(string)
	} else {
		computerNamePrefix = name
	}

	disablePasswordAuthentication := d.Get("disable_password_authentication").(bool)
	networkProfile := &compute.VirtualMachineScaleSetNetworkProfile{
		NetworkInterfaceConfigurations: networkInterfaces,
	}
	priority := compute.VirtualMachinePriorityTypes(d.Get("priority").(string))
	upgradePolicy := compute.UpgradePolicy{
		AutomaticOSUpgradePolicy: automaticOSUpgradePolicy,
		Mode:                     upgradeMode,
	}
	if rollingUpgradePolicy != nil {
		upgradePolicy.RollingUpgradePolicy = &rollingUpgradePolicy.UpgradePolicy
		networkProfile.HealthProbe = &compute.APIEntityReference{
			ID: utils.String(rollingUpgradePolicy.HealthProbeID),
		}
	}

	// TODO: data disks
	dataDisks := make([]compute.VirtualMachineScaleSetDataDisk, 0)

	virtualMachineProfile := compute.VirtualMachineScaleSetVMProfile{
		Priority: priority,
		OsProfile: &compute.VirtualMachineScaleSetOSProfile{
			AdminUsername:      utils.String(d.Get("admin_username").(string)),
			ComputerNamePrefix: utils.String(computerNamePrefix),
			LinuxConfiguration: &compute.LinuxConfiguration{
				DisablePasswordAuthentication: utils.Bool(disablePasswordAuthentication),
				ProvisionVMAgent:              utils.Bool(d.Get("provision_vm_agent").(bool)),
				SSH: &compute.SSHConfiguration{
					PublicKeys: &sshKeys,
				},
			},
			// TODO: secrets
		},
		// TODO: DiagnosticsProfile:
		NetworkProfile: networkProfile,
		StorageProfile: &compute.VirtualMachineScaleSetStorageProfile{
			ImageReference: sourceImageReference,
			OsDisk:         osDisk,
			DataDisks:      &dataDisks,
		},
	}

	if adminPassword, ok := d.GetOk("admin_password"); ok {
		virtualMachineProfile.OsProfile.AdminPassword = utils.String(adminPassword.(string))
	}

	if v, ok := d.GetOk("custom_data"); ok {
		virtualMachineProfile.OsProfile.CustomData = utils.String(v.(string))
	}

	// TODO: confirm this in the API
	if disablePasswordAuthentication && virtualMachineProfile.OsProfile.AdminPassword == nil && len(sshKeys) == 0 {
		return fmt.Errorf("At least one SSH key must be specified if `disable_password_authentication` is enabled")
	}

	if evictionPolicyRaw, ok := d.GetOk("eviction_policy"); ok {
		if virtualMachineProfile.Priority != compute.Low {
			return fmt.Errorf("An `eviction_policy` can only be specified when `priority` is set to `low`")
		}
		virtualMachineProfile.EvictionPolicy = compute.VirtualMachineEvictionPolicyTypes(evictionPolicyRaw.(string))
	} else if priority == compute.Low {
		return fmt.Errorf("An `eviction_policy` must be specified when `priority` is set to `low`")
	}

	props := compute.VirtualMachineScaleSet{
		// TODO: Identity, Plan
		Location: utils.String(location),
		Sku: &compute.Sku{
			Name:     utils.String(d.Get("sku").(string)),
			Capacity: utils.Int64(int64(d.Get("instances").(int))),

			// doesn't appear this can be set to anything else, even Promo machines are Standard
			Tier: utils.String("Standard"),
		},
		Tags: tags.Expand(t),
		VirtualMachineScaleSetProperties: &compute.VirtualMachineScaleSetProperties{
			AdditionalCapabilities:                 additionalCapabilities,
			DoNotRunExtensionsOnOverprovisionedVMs: utils.Bool(d.Get("do_not_run_extensions_on_overprovisioned_machines").(bool)),
			Overprovision:                          utils.Bool(d.Get("overprovision").(bool)),
			SinglePlacementGroup:                   utils.Bool(d.Get("single_placement_group").(bool)),
			VirtualMachineProfile:                  &virtualMachineProfile,
			UpgradePolicy:                          &upgradePolicy,
		},
		Zones: zones,
	}

	if v, ok := d.GetOk("proximity_placement_group_id"); ok {
		props.VirtualMachineScaleSetProperties.ProximityPlacementGroup = &compute.SubResource{
			ID: utils.String(v.(string)),
		}
	}

	if v, ok := d.GetOk("zone_balance"); ok && v.(bool) {
		if len(zonesRaw) == 0 {
			return fmt.Errorf("`zone_balance` can only be set to `true` when zones are specified")
		}

		props.VirtualMachineScaleSetProperties.ZoneBalance = utils.Bool(v.(bool))
	}

	log.Printf("[DEBUG] Creating Linux Virtual Machine Scale Set %q (Resource Group %q)..", name, resourceGroup)
	future, err := client.CreateOrUpdate(ctx, resourceGroup, name, props)
	if err != nil {
		return fmt.Errorf("Error creating Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	log.Printf("[DEBUG] Waiting for Linux Virtual Machine Scale Set %q (Resource Group %q) to be created..", name, resourceGroup)
	if err := future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for creation of Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
	}
	log.Printf("[DEBUG] Virtual Machine Scale Set %q (Resource Group %q) was created", name, resourceGroup)

	log.Printf("[DEBUG] Retrieving Virtual Machine Scale Set %q (Resource Group %q)..", name, resourceGroup)
	resp, err := client.Get(ctx, resourceGroup, name)
	if err != nil {
		return fmt.Errorf("Error retrieving Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	if resp.ID == nil {
		return fmt.Errorf("Error retrieving Linux Virtual Machine Scale Set %q (Resource Group %q): ID was nil", name, resourceGroup)
	}
	d.SetId(*resp.ID)

	// this shouldn't need to go into Update, but let's see
	return resourceArmLinuxVirtualMachineScaleSetRead(d, meta)
}

func resourceArmLinuxVirtualMachineScaleSetUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).compute.VMScaleSetClient
	ctx := meta.(*ArmClient).StopContext

	id, err := computeSvc.ParseVirtualMachineScaleSetResourceID(d.Id())
	if err != nil {
		return err
	}

	name := id.Name
	resourceGroup := id.Base.ResourceGroup

	updateConfig := false

	update := compute.VirtualMachineScaleSetUpdate{
		VirtualMachineScaleSetUpdateProperties: &compute.VirtualMachineScaleSetUpdateProperties{
			VirtualMachineProfile: &compute.VirtualMachineScaleSetUpdateVMProfile{},
		},
	}

	// TODO: secrets
	if d.HasChange("admin_ssh_key") || d.HasChange("custom_data") || d.HasChange("disable_password_authentication") || d.HasChange("provision_vm_agent") {
		updateConfig = true

		sshKeysRaw := d.Get("admin_ssh_key").(*schema.Set).List()
		sshKeys := computeSvc.ExpandSSHKeys(sshKeysRaw)
		update.VirtualMachineProfile.OsProfile = &compute.VirtualMachineScaleSetUpdateOSProfile{
			LinuxConfiguration: &compute.LinuxConfiguration{
				DisablePasswordAuthentication: utils.Bool(d.Get("disable_password_authentication").(bool)),
				ProvisionVMAgent:              utils.Bool(d.Get("provision_vm_agent").(bool)),
				SSH: &compute.SSHConfiguration{
					PublicKeys: &sshKeys,
				},
			},
		}

		if v, ok := d.GetOk("custom_data"); ok {
			update.VirtualMachineProfile.OsProfile.CustomData = utils.String(v.(string))
			// TODO: if we update the customData do we need to cycle the nodes?
		}
	}

	// TODO: or data disk
	if d.HasChange("os_disk") || d.HasChange("source_image_id") || d.HasChange("source_image_reference") {
		updateConfig = true

		// TODO: implement me
		//update.VirtualMachineProfile.StorageProfile = &compute.StorageProfile{}
	}

	if d.HasChange("network_interface") {
		updateConfig = true

		// TODO: implement me
		//update.VirtualMachineProfile.NetworkProfile
	}

	// TODO: diags

	updateInstances := false

	if d.HasChange("tags") {
		updateConfig = true

		update.Tags = tags.Expand(d.Get("tags").(map[string]interface{}))
	}

	if updateConfig {
		log.Printf("[DEBUG] Updating Linux Virtual Machine Scale Set %q (Resource Group %q)..", name, resourceGroup)
		future, err := client.Update(ctx, resourceGroup, name, update)
		if err != nil {
			return fmt.Errorf("Error updating Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
		}

		log.Printf("[DEBUG] Waiting for update of Linux Virtual Machine Scale Set %q (Resource Group %q)..", name, resourceGroup)
		if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
			return fmt.Errorf("Error waiting for update of Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
		}
		log.Printf("[DEBUG] Updated Linux Virtual Machine Scale Set %q (Resource Group %q).", name, resourceGroup)
	}

	// TODO: delta updates

	// TODO: if rolling the image and there's a manual healthcheck should we cycle this here? flag?
	// client.Reimage()
	// ConvertToSinglePlacementGroup

	// if we update the SKU, we also need to subsequently roll the instances using the `UpdateInstances` API
	if updateInstances {
		// TODO: retrieve all the instances, then roll them one at a time
		// looks like this can be found in the VMScaleSetsVMClient
		// providers/Microsoft.Compute/virtualMachineScaleSets/tom-dev6/virtualMachines which exposes 'isLatestModel'
		// client.UpdateInstances()
	}

	return resourceArmLinuxVirtualMachineScaleSetRead(d, meta)
}

func resourceArmLinuxVirtualMachineScaleSetRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).compute.VMScaleSetClient
	ctx := meta.(*ArmClient).StopContext

	id, err := computeSvc.ParseVirtualMachineScaleSetResourceID(d.Id())
	if err != nil {
		return err
	}

	name := id.Name
	resourceGroup := id.Base.ResourceGroup

	resp, err := client.Get(ctx, resourceGroup, name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[DEBUG] Linux Virtual Machine Scale Set %q was not found in Resource Group %q - removing from state!", name, resourceGroup)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error retrieving Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	d.Set("name", name)
	d.Set("resource_group_name", resourceGroup)
	if location := resp.Location; location != nil {
		d.Set("location", azure.NormalizeLocation(*location))
	}

	var skuName *string
	var instances int
	if resp.Sku != nil {
		skuName = resp.Sku.Name
		if resp.Sku.Capacity != nil {
			instances = int(*resp.Sku.Capacity)
		}
	}
	d.Set("instances", instances)
	d.Set("sku", skuName)

	if resp.VirtualMachineScaleSetProperties == nil {
		return fmt.Errorf("Error retrieving Linux Virtual Machine Scale Set %q (Resource Group %q): `properties` was nil", name, resourceGroup)
	}
	props := *resp.VirtualMachineScaleSetProperties

	if err := d.Set("additional_capabilities", computeSvc.FlattenVirtualMachineScaleSetAdditionalCapabilities(props.AdditionalCapabilities)); err != nil {
		return fmt.Errorf("Error setting `additional_capabilities`: %+v", props.AdditionalCapabilities)
	}
	d.Set("do_not_run_extensions_on_overprovisioned_machines", props.DoNotRunExtensionsOnOverprovisionedVMs)
	d.Set("overprovision", props.Overprovision)
	if group := props.ProximityPlacementGroup; group != nil {
		d.Set("proximity_placement_group_id", group.ID)
	}
	d.Set("single_placement_group", props.SinglePlacementGroup)
	d.Set("unique_id", props.UniqueID)
	d.Set("zone_balance", props.ZoneBalance)

	var healthProbeId *string
	if profile := props.VirtualMachineProfile; profile != nil {
		d.Set("eviction_policy", string(profile.EvictionPolicy))
		d.Set("priority", string(profile.Priority))

		if storageProfile := profile.StorageProfile; storageProfile != nil {
			if d.Set("os_disk", computeSvc.FlattenVirtualMachineScaleSetOSDisk(storageProfile.OsDisk)); err != nil {
				return fmt.Errorf("Error setting `os_disk`: %+v", err)
			}

			if d.Set("source_image_reference", computeSvc.FlattenVirtualMachineScaleSetSourceImageReference(storageProfile.ImageReference)); err != nil {
				return fmt.Errorf("Error setting `source_image_reference`: %+v", err)
			}

			var storageImageId string
			if storageProfile.ImageReference != nil && storageProfile.ImageReference.ID != nil {
				storageImageId = *storageProfile.ImageReference.ID
			}
			d.Set("source_image_id", storageImageId)
		}

		if osProfile := profile.OsProfile; osProfile != nil {
			// admin_password isn't returned, but it's a top level field so we can ignore it without consequence
			d.Set("admin_username", osProfile.AdminUsername)
			d.Set("computer_name_prefix", osProfile.ComputerNamePrefix)

			if linux := osProfile.LinuxConfiguration; linux != nil {
				d.Set("disable_password_authentication", linux.DisablePasswordAuthentication)
				d.Set("provision_vm_agent", linux.ProvisionVMAgent)

				flattenedSshKeys, err := computeSvc.FlattenSSHKeys(linux.SSH)
				if err != nil {
					return fmt.Errorf("Error flattening `admin_ssh_key`: %+v", err)
				}
				if err := d.Set("admin_ssh_key", *flattenedSshKeys); err != nil {
					return fmt.Errorf("Error setting `admin_ssh_key`: %+v", err)
				}
			}
		}

		if nwProfile := profile.NetworkProfile; nwProfile != nil {
			flattenedNics := computeSvc.FlattenVirtualMachineScaleSetNetworkInterface(nwProfile.NetworkInterfaceConfigurations)
			if d.Set("network_interface", flattenedNics); err != nil {
				return fmt.Errorf("Error setting `network_interface`: %+v", err)
			}

			if nwProfile.HealthProbe != nil {
				healthProbeId = nwProfile.HealthProbe.ID
			}
		}
	}

	if policy := props.UpgradePolicy; policy != nil {
		d.Set("upgrade_mode", string(policy.Mode))

		flattenedAutomatic := computeSvc.FlattenVirtualMachineScaleSetAutomaticOSUpgradePolicy(policy.AutomaticOSUpgradePolicy)
		if err := d.Set("automatic_os_upgrade_policy", flattenedAutomatic); err != nil {
			return fmt.Errorf("Error setting `automatic_os_upgrade_policy`: %+v", err)
		}

		flattenedRolling := computeSvc.FlattenVirtualMachineScaleSetRollingUpgradePolicy(policy.RollingUpgradePolicy, healthProbeId)
		if err := d.Set("rolling_upgrade_policy", flattenedRolling); err != nil {
			return fmt.Errorf("Error setting `rolling_upgrade_policy`: %+v", err)
		}
	}

	if err := d.Set("zones", resp.Zones); err != nil {
		return fmt.Errorf("Error setting `zones`: %+v", err)
	}

	return tags.FlattenAndSet(d, resp.Tags)
}

func resourceArmLinuxVirtualMachineScaleSetDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).compute.VMScaleSetClient
	ctx := meta.(*ArmClient).StopContext

	id, err := computeSvc.ParseVirtualMachineScaleSetResourceID(d.Id())
	if err != nil {
		return err
	}

	name := id.Name
	resourceGroup := id.Base.ResourceGroup

	future, err := client.Delete(ctx, resourceGroup, name)
	if err != nil {
		return fmt.Errorf("Error deleting Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	if err := future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for deletion of Linux Virtual Machine Scale Set %q (Resource Group %q): %+v", name, resourceGroup, err)
	}

	return nil
}