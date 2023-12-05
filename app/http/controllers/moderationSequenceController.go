package controllers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/62teknologi/62seahorse/62golib/utils"
	"github.com/62teknologi/62seahorse/app/app_constant"
	"github.com/62teknologi/62seahorse/config"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ModerationSequenceController struct {
	SingularName        string
	PluralName          string
	SingularLabel       string
	PluralLabel         string
	Table               string
	PrefixSingularName  string
	PrefixPluralName    string
	PrefixSingularLabel string
	PrefixPluralLabel   string
	PrefixTable         string
	SuffixSingularName  string
	SuffixPluralName    string
	SuffixSingularLabel string
	SuffixTable         string
}

func (ctrl *ModerationSequenceController) Init(ctx *gin.Context) {
	ctrl.SingularName = utils.Pluralize.Singular(ctx.Param("table"))
	ctrl.PluralName = utils.Pluralize.Plural(ctx.Param("table"))
	ctrl.SingularLabel = ctrl.SingularName
	ctrl.PluralLabel = ctrl.PluralName
	ctrl.Table = ctrl.PluralName
	ctrl.PrefixSingularName = utils.Pluralize.Singular(config.Data.Prefix)
	ctrl.PrefixPluralName = utils.Pluralize.Plural(config.Data.Prefix)
	ctrl.PrefixSingularLabel = ctrl.PrefixSingularName
	ctrl.PrefixTable = ctrl.PrefixPluralName
	ctrl.SuffixSingularName = utils.Pluralize.Singular(config.Data.Suffix)
	ctrl.SuffixPluralName = utils.Pluralize.Plural(config.Data.Suffix)
	ctrl.SuffixSingularLabel = ctrl.SuffixSingularName
	ctrl.SuffixTable = ctrl.SuffixPluralName
}

func (ctrl ModerationSequenceController) Moderate(ctx *gin.Context) {
	ctrl.Init(ctx)

	transformer, err := utils.JsonFileParser(config.Data.SettingPath + "/transformers/request/moderate.json")

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, utils.ResponseData("error", err.Error(), nil))
		return
	}

	input := utils.ParseForm(ctx)

	if validation, err := utils.Validate(input, transformer); err {
		ctx.JSON(http.StatusBadRequest, utils.ResponseData("error", "validation", validation.Errors))
		return
	}

	utils.MapValuesShifter(transformer, input)
	utils.MapNullValuesRemover(transformer)

	if err = utils.DB.Transaction(func(tx *gorm.DB) error {
		moderationSequence := make(map[string]any)
		if err := tx.Table("mod_" + ctrl.PrefixSingularName+"_" + ctrl.SuffixTable).Where("id = ?", ctx.Param("id")).Take(&moderationSequence).Error; err != nil {
			return err
		}

		if fmt.Sprintf("%v", moderationSequence["result"]) != fmt.Sprintf("%v", app_constant.Pending) {
			return errors.New("Moderation Sequence must be pending")
		}

		moderationSequenceUser := make(map[string]any)
		if err := tx.Table("mod_" + ctrl.PrefixSingularName+"_users").Where("moderation_sequence_id = ?", moderationSequence["id"]).Where("user_id = ?", transformer["moderator_id"]).Take(&moderationSequenceUser).Error; err != nil {
			return errors.New("moderator is not in this moderation sequence")
		}

		moderationSequence["is_current"] = false
		moderationSequence["result"] = transformer["result"]
		moderationSequence["remarks"] = transformer["remarks"]
		moderationSequence["file_id"] = transformer["file_id"]

		moderation := make(map[string]any)
		if err := tx.Table("mod_" + ctrl.PrefixTable).Where("id = ?", moderationSequence["moderation_id"]).Take(&moderation).Error; err != nil {
			return err
		}

		if moderation["step_current"] == nil {
			moderation["step_current"] = 1
		} else {
			moderation["step_current"] = utils.ConvertToInt(moderation["step_current"]) + 1
		}

		if fmt.Sprintf("%v", moderation["step_current"]) != fmt.Sprintf("%v", moderationSequence["step"]) && fmt.Sprintf("%v", moderation["is_in_order"]) == fmt.Sprintf("%v", 1) {
			return errors.New("Moderation sequence is not current")
		}

		if fmt.Sprintf("%v", moderation["result"]) == fmt.Sprintf("%v", app_constant.Approve) ||
			fmt.Sprintf("%v", moderation["result"]) == fmt.Sprintf("%v", app_constant.Reject) {
			return errors.New("Moderation is already finished")
		}

		moderation["last_moderation_sequence_id"] = moderationSequence["id"]
		unModeratedSequences := make([]map[string]any, 0)

		if err := tx.Table("mod_" + ctrl.PrefixSingularName+"_" + ctrl.SuffixTable).Where("moderation_id = ?", moderation["id"]).Where("result = ?", app_constant.Pending).Where("id != ?", moderationSequence["id"]).Find(&unModeratedSequences).Error; err != nil {
			return err
		}

		if fmt.Sprintf("%v", moderation["is_in_order"]) != fmt.Sprintf("%v", 1) {
			moderationSequence["step"] = moderation["step_current"]
		} else {
			if len(unModeratedSequences) > 0 {
				if err = tx.Table("mod_" + ctrl.PrefixSingularName+"_" + ctrl.SuffixTable).Where("moderation_id = ?", moderation["id"]).Where("step = ?", utils.ConvertToInt(moderation["step_current"])+1).Update("is_current", true).Error; err != nil {
					return err
				}
			}
		}

		moderationSequence["is_current"] = false

		if fmt.Sprintf("%v", transformer["result"]) == fmt.Sprintf("%v", app_constant.Approve) {
			if len(unModeratedSequences) == 0 {
				moderation["status"] = app_constant.Approve
			} else {
				// convert moderationSequence["step"] to int and add 1
				moderation["status"] = app_constant.Pending
			}
		} else {
			moderation["status"] = moderationSequence["result"]
		}

		moderationSequence["moderator_id"] = transformer["moderator_id"]

		if err := tx.Table("mod_" + ctrl.PrefixSingularName+"_" + ctrl.SuffixTable).Where("id = ?", moderationSequence["id"]).Updates(&moderationSequence).Error; err != nil {
			return err
		}

		if err := tx.Table("mod_" + ctrl.PrefixTable).Where("id = ?", moderation["id"]).Updates(&moderation).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		ctx.JSON(http.StatusBadRequest, utils.ResponseData("error", err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, utils.ResponseData("success", "create "+ctrl.PrefixSingularLabel+" "+ctrl.PrefixTable+" success", transformer))
}

func (ctrl ModerationSequenceController) UpdateModerator(ctx *gin.Context) {
	ctrl.Init(ctx)

	transformer, err := utils.JsonFileParser(config.Data.SettingPath + "/transformers/request/" + ctrl.PluralName + "/moderator.json")

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, utils.ResponseData("error", err.Error(), nil))
		return
	}

	input := utils.ParseForm(ctx)

	if validation, err := utils.Validate(input, transformer); err {
		ctx.JSON(http.StatusBadRequest, utils.ResponseData("error", "validation", validation.Errors))
		return
	}

	utils.MapValuesShifter(transformer, input)
	utils.MapNullValuesRemover(transformer)

	if err = utils.DB.Transaction(func(tx *gorm.DB) error {
		moderationSequence := make(map[string]any)
		if err := tx.Table("mod_" + ctrl.PrefixSingularName+"_").Where("id = ?", ctx.Param("id")).Take(&moderationSequence).Error; err != nil {
			return err
		}

		// delete all moderation_sequence_users
		if err := tx.Table(ctrl.PrefixSingularName+"_sequence_users").Where("moderation_sequence_id = ?", moderationSequence["id"]).Delete(&moderationSequence).Error; err != nil {
			return err
		}

		// insert new moderation_sequence_users
		createModerationSequenceUser := []map[string]any{}
		for _, v := range transformer["user_ids"].([]any) {
			createModerationSequenceUser = append(createModerationSequenceUser, map[string]any{
				"moderation_sequence_id": moderationSequence["id"],
				"user_id":                v,
			})
		}

		if err = tx.Table(ctrl.PrefixSingularName + "_sequence_users").Create(&createModerationSequenceUser).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		ctx.JSON(http.StatusBadRequest, utils.ResponseData("error", err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, utils.ResponseData("success", "update "+ctrl.PrefixSingularLabel+" "+ctrl.PrefixTable+" success", transformer))
}
