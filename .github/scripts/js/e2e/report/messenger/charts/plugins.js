// Copyright 2026 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

const STACK_LABEL_INLINE_MIN_PX = 34;

function isStackedChart(chart, isHorizontal) {
  const axisKey = isHorizontal ? "x" : "y";
  return Boolean(chart.options.scales?.[axisKey]?.stacked);
}

function drawHorizontalInline(ctx, props, label, isStacked, barWidth) {
  ctx.textAlign =
    isStacked && barWidth > STACK_LABEL_INLINE_MIN_PX ? "center" : "left";
  ctx.fillText(
    label,
    isStacked && barWidth > STACK_LABEL_INLINE_MIN_PX
      ? (props.x + props.base) / 2
      : props.x + 6,
    props.y
  );
}

function drawVerticalAbove(ctx, props, label) {
  ctx.textAlign = "center";
  ctx.fillText(label, props.x, props.y - 8);
}

function drawValueLabels(chart, _args, options) {
  const { ctx, data } = chart;
  const formatter = options && options.formatter;
  if (typeof formatter !== "function") {
    return;
  }

  ctx.save();
  ctx.font = "12px sans-serif";
  ctx.fillStyle = "#24292f";
  ctx.textBaseline = "middle";

  chart.getSortedVisibleDatasetMetas().forEach((meta) => {
    meta.data.forEach((element, dataIndex) => {
      const rawValue = data.datasets[meta.index].data[dataIndex];
      if (!rawValue) {
        return;
      }

      const label = formatter(rawValue, {
        chart,
        dataIndex,
        datasetIndex: meta.index,
      });
      if (!label) {
        return;
      }

      const props = element.getProps(["x", "y", "base"], true);
      const isHorizontal = chart.options.indexAxis === "y";
      const isStacked = isStackedChart(chart, isHorizontal);

      if (isHorizontal) {
        const barWidth = Math.abs(props.x - props.base);
        drawHorizontalInline(ctx, props, label, isStacked, barWidth);
        return;
      }

      drawVerticalAbove(ctx, props, label);
    });
  });

  ctx.restore();
}

const valueLabelsPlugin = {
  id: "valueLabels",
  afterDatasetsDraw: drawValueLabels,
};

module.exports = {
  STACK_LABEL_INLINE_MIN_PX,
  valueLabelsPlugin,
  drawValueLabels,
};
